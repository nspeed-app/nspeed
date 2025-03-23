package metrics

import (
	"bytes"
	"fmt"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/rs/zerolog/log"
	"golang.org/x/term"
	"nspeed.app/helper/humanize"
	"nspeed.app/helper/str"
)

// generate a report per batch (sorted), filtering failed jobs
func GenerateTextReport(wcWorkers *Metrics, showGroup bool) (out string) {
	if wcWorkers == nil {
		log.Info().Msgf("generating text report: no data")
		return
	}
	log.Info().Msgf("generating text report for %d metric(s) in %d batch()", len(wcWorkers.Metrics), len(wcWorkers.Batches))
	for _, wm := range wcWorkers.Metrics {
		if wm.Error != nil && !wm.Valid {
			out += fmt.Sprintf("batch %d, job %s, error = %s\n", wm.Batch, wm.ID, wm.Error)
			continue
		}
	}
	// loop over batch (dont "range batchs" to avoid bad order)
	for idx, batch := range wcWorkers.Batches {
		s := GenerateTextReportBatch(idx, wcWorkers, showGroup)
		if s != "" {
			out += fmt.Sprintf("batch %s:\n", batch) + s + "\n"
		}
	}
	return
}

// generate a tabular formatted text report
func GenerateTextReportBatch(batch int, wcWorkers *Metrics, showGroup bool) string {

	if wcWorkers == nil {
		return ""
	}

	if len(wcWorkers.Metrics) == 0 {
		return ""
	}
	var out bytes.Buffer

	var elapsedTime time.Duration
	var totalRead int64
	var totalWrite int64
	groups := make(map[string]*reportGroup)

	w := tabwriter.NewWriter(&out, 0, 2, 1, ' ', tabwriter.AlignRight|tabwriter.Debug)

	// header line
	fmt.Fprintf(w, "Id\tRead speed\tWrite speed\tTime\tBytes read\tBytes written\tcommand\n")
	// use this function to add lines
	addLine := func(w *tabwriter.Writer, id string, elapsedTime time.Duration, totalRead int64, totalWrite int64, target string) {
		fmt.Fprintf(w, "%s", fmt.Sprintf("%s\t%s\t%s\t%.2f\t%sB\t%sB\t%s\n",
			id,
			humanize.FormatBitperSecond(totalRead, elapsedTime),
			humanize.FormatBitperSecond(totalWrite, elapsedTime),
			elapsedTime.Seconds(),
			humanize.ByteCountDecimal(totalRead),
			humanize.ByteCountDecimal(totalWrite),
			target))
	}

	hasGroup := false
	lines := 0
	for _, wm := range wcWorkers.Metrics {
		if wm.Batch != batch {
			continue
		}
		if !wm.Valid {
			continue
		}
		if wm.Error != nil {
			log.Warn().Err(wm.Error).Msgf("job %s ended", wm.ID)
			continue
		}
		inGroup := false
		totalRead += wm.TotalRead
		totalWrite += wm.TotalWrite
		if wm.ElapsedTime > elapsedTime {
			elapsedTime = wm.ElapsedTime
		}
		if wm.GroupID != wm.ID {
			inGroup = true
			hasGroup = true
			g, ok := groups[wm.GroupID]
			if !ok {
				g = &reportGroup{}
				groups[wm.GroupID] = g
				g.Command = wm.Command
			}
			g.Instances++
			g.TotalRead += wm.TotalRead
			g.TotalWrite += wm.TotalWrite
			if wm.ElapsedTime > g.ElapsedTime {
				g.ElapsedTime = wm.ElapsedTime
			}
		}
		if !inGroup || showGroup {
			addLine(w, wm.GroupID, wm.ElapsedTime, wm.TotalRead, wm.TotalWrite, wm.Command)
			lines++
		}
	}
	if showGroup && hasGroup {
		fmt.Fprintf(w, "Groups:\t\t\t\t\t\t\n")
	}
	for gid, g := range groups {
		addLine(w, gid, g.ElapsedTime, g.TotalRead, g.TotalWrite, strconv.Itoa(g.Instances)+" x "+g.Command)
		lines++
	}

	if totalRead == 0 && totalWrite == 0 {
		fmt.Fprintf(w, "no metrics to report")
	} else if lines > 1 {
		fmt.Fprintf(w, "\t\t\t\t\t\t\n")
		addLine(w, "Average", elapsedTime, totalRead, totalWrite, "")
	}
	w.Flush()
	return out.String()
}

// generate a report per batch (sorted), filtering failed jobs, console / pretty print table
func GenerateTableReport(f *str.SmartFile, wcWorkers *Metrics, showGroup bool) error {
	if wcWorkers == nil {
		log.Info().Msgf("generating text report: no data")
		return nil
	}
	log.Info().Msgf("generating text report for %d metric(s) in %d batch()", len(wcWorkers.Metrics), len(wcWorkers.Batches))
	for _, wm := range wcWorkers.Metrics {
		if wm.Error != nil && !wm.Valid {
			_, err := fmt.Fprintf(f, "batch %d, job %s, error = %s\n", wm.Batch, wm.ID, wm.Error)
			if err != nil {
				return fmt.Errorf("can't write to console: %w", err)
			}
			continue
		}
	}
	w, h, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return fmt.Errorf("can't get size of terminal: %w", err)
	}
	log.Debug().Msgf("tarminal size is %dx%d", w, h)
	// loop over batch (dont "range batchs" to avoid bad order)
	for idx, batch := range wcWorkers.Batches {
		s := GenerateTableReportBatch(f, w, idx, wcWorkers, showGroup)
		if s != "" {
			_, err := fmt.Fprintf(f, "batch %s:\n%s", batch, s)
			if err != nil {
				return fmt.Errorf("can't write to console: %w", err)
			}
		}
	}
	return nil
}

// generate a tabular formatted text report of given width
func GenerateTableReportBatch(f *str.SmartFile, width int, batch int, wcWorkers *Metrics, showGroup bool) string {

	if wcWorkers == nil {
		return ""
	}

	if len(wcWorkers.Metrics) == 0 {
		return ""
	}
	var elapsedTime time.Duration
	var totalRead int64
	var totalWrite int64
	groups := make(map[string]*reportGroup)

	t := table.New().
		Border(lipgloss.NormalBorder()).
		Width(width).
		Wrap(true).
		// header line
		Headers("Id", "Read speed", "Write speed", "Time", "Bytes read", "Bytes written", "command")

	// use this function to add lines
	addLine := func(id string, elapsedTime time.Duration, totalRead int64, totalWrite int64, target string) {
		t.Row(id,
			humanize.FormatBitperSecond(totalRead, elapsedTime),
			humanize.FormatBitperSecond(totalWrite, elapsedTime),
			strconv.FormatFloat(elapsedTime.Seconds(), 'f', 2, 64),
			humanize.ByteCountDecimal(totalRead),
			humanize.ByteCountDecimal(totalWrite),
			target)
	}

	hasGroup := false
	lines := 0
	for _, wm := range wcWorkers.Metrics {
		if wm.Batch != batch {
			continue
		}
		if !wm.Valid {
			continue
		}
		if wm.Error != nil {
			log.Warn().Err(wm.Error).Msgf("job %s ended", wm.ID)
			continue
		}
		inGroup := false
		totalRead += wm.TotalRead
		totalWrite += wm.TotalWrite
		if wm.ElapsedTime > elapsedTime {
			elapsedTime = wm.ElapsedTime
		}
		if wm.GroupID != wm.ID {
			inGroup = true
			hasGroup = true
			g, ok := groups[wm.GroupID]
			if !ok {
				g = &reportGroup{}
				groups[wm.GroupID] = g
				g.Command = wm.Command
			}
			g.Instances++
			g.TotalRead += wm.TotalRead
			g.TotalWrite += wm.TotalWrite
			if wm.ElapsedTime > g.ElapsedTime {
				g.ElapsedTime = wm.ElapsedTime
			}
		}
		if !inGroup || showGroup {
			addLine(wm.GroupID, wm.ElapsedTime, wm.TotalRead, wm.TotalWrite, wm.Command)
			lines++
		}
	}
	if showGroup && hasGroup {
		t.Row("Groups:")
	}
	for gid, g := range groups {
		addLine(gid, g.ElapsedTime, g.TotalRead, g.TotalWrite, strconv.Itoa(g.Instances)+" x "+g.Command)
		lines++
	}

	if totalRead == 0 && totalWrite == 0 {
		t.Row("no metrics to report")
	} else if lines > 1 {
		t.Row()
		addLine("Average", elapsedTime, totalRead, totalWrite, "")
	}
	return t.String()
}
