package nspeed

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"text/template"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"nspeed.app/humanize"
	"nspeed.app/str"
	"nspeed.app/web"
)

// commands
type CommandID int

const (
	getCommand       = "get"
	putCommand       = "put"
	postCommand      = "post"
	serveCommand     = "server"
	ciphersCommand   = "ciphers"
	apiCommand       = "api"
	benchmarkCommand = "bench"
	fromCommand      = "from"
	thenCommand      = "then"
	uiCommand        = "ui"
)

// const latencyCommand = "latency"
// const tracerouteCommand = "traceroute"

// supported commands, expand has needed
// Set is a set number. Commands in the same set can be mixed together.
// Commands in the "0" set can only be called alone and once
// FromAPI: true is command can be called using the API
// Parse: function to parse the command in command line arguments. see ParseCommand()
type Command struct {
	Name    string
	Set     int
	FromAPI bool
	Parse   func(command string, cmdargs []string, jl *JobList, output io.Writer) ([]string, error)
}

// list of supported commands.
var Commands = map[string]Command{
	getCommand:       {getCommand, 1, true, CmdClientParseGet},
	putCommand:       {putCommand, 1, true, CmdClientParsePutPost},
	postCommand:      {postCommand, 1, true, CmdClientParsePutPost},
	serveCommand:     {serveCommand, 1, true, CmdServerParse},
	ciphersCommand:   {ciphersCommand, 0, false, CmdCiphersParse},
	apiCommand:       {apiCommand, 1, false, CmdAPIParse},
	benchmarkCommand: {benchmarkCommand, 1, true, nil},
	fromCommand:      {fromCommand, 1, true, nil},
	thenCommand:      {thenCommand, 1, true, nil},
	uiCommand:        {uiCommand, 0, false, CmdUIParse},
}

var CommandDescriptions = map[string]string{
	getCommand:       "perform a transfer to measure download speed (default HTTP GET)",
	putCommand:       "perform a transfer to measure upload speed (default HTTP PUT)",
	postCommand:      "perform an HTTP POST command to measure upload speed",
	serveCommand:     "launch a nspeed server (default HTTP server)",
	ciphersCommand:   "list cipher suites supported by the target server(s)",
	apiCommand:       "enable an API endpoint",
	benchmarkCommand: "perform preselected commands",
	fromCommand:      "read command from file (use - for stdin)",
	thenCommand:      "executes all previous commands then continue",
	uiCommand:        "open a web user interface",
}

// global flags
var OptTLSSelf = false

// find command ,try full match first then partial unless ambiguous
func IsCommand(cmd string) (Command, error) {
	c, ok := Commands[cmd]
	if ok {
		return c, nil
	}

	found := ""
	for _, v := range Commands {
		if strings.HasPrefix(v.Name, cmd) {
			if found != "" {
				return Command{}, errors.New("ambiguous command name: " + cmd)
			}
			found = v.Name

		}
	}
	if found != "" {
		return IsCommand(found)
	}
	return Command{}, errors.New("command not found: " + cmd)
}

// ParseCommand parses cmdargs for command and its arguments.
//
// Commands without a dedicated Parse() method are parsed here.
//
// Parsed jobs are added to the job list jl.
//
// Returns the remainder of cmdargs.
func ParseCommand(command string, cmdargs []string, jl *JobList, output io.Writer) ([]string, error) {
	cs, ok := Commands[command]
	if !ok {
		return cmdargs, errors.New("unknown command")
	}
	if cs.Parse != nil {
		return cs.Parse(command, cmdargs, jl, output)
	}
	// commands without Parse() method
	switch command {
	case benchmarkCommand:
		fs := flag.NewFlagSet(command, flag.ContinueOnError)

		fs.Usage = func() {
			fmt.Fprintf(fs.Output(), "Usage of %s: ", benchmarkCommand)
			fmt.Fprintf(fs.Output(), "  %s [options] benchmark[,benchmark]\n", benchmarkCommand)
			fmt.Fprintf(fs.Output(), "\nAvailable benchmark:\n")

			for _, k := range benchKeys {
				fmt.Fprintf(fs.Output(), "  %-4s: %s\n", k, benchmarksTemplates[k].Help)
			}

			fmt.Fprintf(fs.Output(), "\nAvailable options:\n")
			fs.PrintDefaults()
			fmt.Fprintf(fs.Output(), "\n")
		}
		useIPv4 := fs.Bool("4", false, "use IPv4 only")
		useIPv6 := fs.Bool("6", false, "use IPv6 only")
		nStreams := fs.Int("n", 1, "number of streams")
		size := fs.String("s", "20g", "number of bytes")
		timeOut := fs.Uint("m", 0, "maximum duration in seconds the command is allowed to take")
		// id := fs.String("id", "", "name/id of this command")

		if err := fs.Parse(cmdargs); err != nil {
			return cmdargs, err
		}
		// force IPv4 or IPv6
		if *useIPv4 && *useIPv6 {
			fs.Usage()
			return cmdargs, fmt.Errorf("specify IPv4 or IPv6 but not both")
		}
		argsLine := ""
		if *timeOut > 0 {
			argsLine = fmt.Sprintf("-m %d", *timeOut)
		}
		if *useIPv6 {
			argsLine += " -6"
		}
		if *useIPv4 {
			argsLine += " -4"
		}

		// we parse the size just to check it's valid
		_, err := humanize.ParseByteUnits(*size)
		if err != nil {
			return cmdargs, err
		}

		args := fs.Args()
		if len(args) < 1 {
			fs.Usage()
			return args, fmt.Errorf("missing benchmark parameters")
		}
		benchs := strings.Split(args[0], ",")
		args = args[1:]

		values := BenchmarkArgs{
			SArgs: argsLine,
			CArgs: argsLine,
			N:     *nStreams,
			Size:  *size,
			ID:    jl.NextBatchID(),
		}
		for _, arg := range benchs {
			ct, ok := benchmarksTemplates[arg]
			if !ok {
				log.Fatal().Msgf("unknown benchmark: '%s'", arg)
			}
			tpl := template.New("T")
			t := template.Must(tpl.Parse(ct.Tpl))

			values.ID = jl.NextBatchID()
			values.CArgs = argsLine + ` -id "` + jl.MakeID(ct.Name, " ") + `"`
			var c bytes.Buffer
			if err := t.Execute(&c, values); err != nil {
				log.Fatal().Err(err).Send()
			}
			cmd := str.Fields(c.String())
			log.Info().Msgf("benchmark: %s", cmd)
			err := ParseCommands(cmd, jl, output, false)
			if err != nil {
				return args, err
			}
		}
		return args, nil
	case fromCommand:
		fs := flag.NewFlagSet(command, flag.ContinueOnError)

		kvs := make(kvMap)
		fs.Var(&kvs, "kv", "template field name/value (repeatable)")

		fs.Usage = func() {
			fmt.Fprintf(fs.Output(), "Usage of %s: ", fromCommand)
			fmt.Fprintf(fs.Output(), "  %s filename\n", fromCommand)
			fs.PrintDefaults()
			fmt.Fprintf(fs.Output(), "\n")
		}

		if err := fs.Parse(cmdargs); err != nil {
			return cmdargs, err
		}
		args := fs.Args()
		if len(args) < 1 {
			fs.Usage()
			return args, errors.New("missing filename")
		}
		uri := args[0]
		args = args[1:]
		log.Debug().Msgf("fromCommand, url/file to process = %s", uri)

		// process file
		cmdFile := io.ReadCloser(os.Stdin)
		if uri != "-" {
			var err error
			cmdFile, err = web.OpenURI(uri)
			if err != nil {
				return args, fmt.Errorf("can't open uri %s (%w)", uri, err)
			}
		}
		{
			//nolint: errcheck
			defer cmdFile.Close()

			//nolint: errcheck
			tplfile := new(bytes.Buffer)
			_, err := tplfile.ReadFrom(cmdFile)
			if err != nil {
				return args, fmt.Errorf("can't read uri %s (%w)", uri, err)
			}

			tpl := template.New(uri).Option("missingkey=zero")
			t, err := tpl.Parse(tplfile.String())
			if err != nil {
				return args, fmt.Errorf("can't parse uri %s (%w)", uri, err)
			}

			var c bytes.Buffer
			if err := t.Execute(&c, kvs); err != nil {
				return args, fmt.Errorf("can't templatize uri %s (%w)", uri, err)
			}

			fmt.Println("\n-----", c.String(), "-----")

			cmdScan := bufio.NewScanner(bytes.NewReader(c.Bytes()))

			log.Warn().Msgf("reading commands from %s", uri)
			lineNumber := 1
			for cmdScan.Scan() {
				rline := cmdScan.Text()
				if strings.TrimSpace(rline) == "" {
					lineNumber++
					continue
				}
				line, err := str.ToArgv(rline)
				if err != nil {
					return args, err
				}
				name := fmt.Sprintf("line %d of %s", lineNumber, uri)

				if jl.currentBatchEmpty {
					jl.Batches[len(jl.Batches)-1].Name = name
				} else {
					err = jl.AddBatch(&Batch{Name: name})
				}
				if err != nil {
					return args, err
				}
				err = ParseCommands(line, jl, output, false)
				if err != nil {
					return args, err
				}
				lineNumber++
			}
			// nothing found in the file
			if lineNumber == 1 {
				return args, fmt.Errorf("no command found in file %s", uri)
			}
		}
		return args, nil
	case thenCommand:
		fs := flag.NewFlagSet(command, flag.ContinueOnError)

		fs.Usage = func() {
			fmt.Fprintf(fs.Output(), "Usage of %s: ", thenCommand)
			fmt.Fprintf(fs.Output(), "  %s [options]\n", thenCommand)
			fmt.Fprintf(fs.Output(), "\nAvailable options:\n")
			fs.PrintDefaults()
			fmt.Fprintf(fs.Output(), "\n")
		}
		batch := Batch{}
		fs.DurationVar(&batch.PreDelay, "pre", 0, "duration to wait before starting commands")
		fs.DurationVar(&batch.PostDelay, "post", 0, "duration to wait after commands ended")
		fs.StringVar(&batch.Name, "n", "", "name of this batch")
		if err := fs.Parse(cmdargs); err != nil {
			return cmdargs, err
		}
		err := jl.AddBatch(&batch)
		if err != nil {
			return cmdargs, err
		}
		args := fs.Args()
		return args, nil
	default:
		return cmdargs, errors.New("unknown command")
	}
}

// Thx to https://github.com/artyom for hints for ParseCommands

// ParseCommands parses multiple commands
//
// make sure global args are parsed first
//
// this allow parsing of the form:
//
//	program [global flags] command1 .... commandN
//
// where commandX = command [local flags] [args]
func ParseCommands(rawArgs []string, joblist *JobList, output io.Writer, fromAPI bool) error {
	args := rawArgs
	cmdSet := -1 // no set yet

	for len(args) != 0 {
		command := args[0]
		if len(command) > 0 && command[0] == '#' {
			return nil
		}
		args = args[1:]
		cmd, err := IsCommand(command)
		if err != nil {
			return err
		}
		if cmd.Set == 0 && cmdSet != -1 {
			return errors.New("only one command of this type is allowed")
		}
		if cmdSet == -1 {
			cmdSet = cmd.Set
		}
		if cmdSet != cmd.Set {
			return fmt.Errorf("cannot mix this command (%s) with the previous one(s)", cmd.Name)
		}
		if fromAPI && !cmd.FromAPI {
			return fmt.Errorf("this command (%s) can not be called from the API", cmd.Name)
		}
		reminder, err := ParseCommand(cmd.Name, args, joblist, output)
		if err != nil {
			return err
		}
		args = reminder
	}
	return nil
}

// LogMemUsage displays runtime.ReadMemStats() in user friendly format
func LogMemUsage(log *zerolog.Logger, mess string, v ...any) {
	if log == nil {
		return
	}
	if log.GetLevel() > zerolog.DebugLevel {
		return
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// For info on each, see: https://golang.org/pkg/runtime/#MemStats
	log.Debug().Msgf("%s (Alloc = %vB TotalAlloc = %vB Sys = %vB NumGC = %v)",
		fmt.Sprintf(mess, v...),
		humanize.FormatByteDecimalUnits(int64(m.Alloc)),
		humanize.FormatByteDecimalUnits(int64(m.TotalAlloc)),
		humanize.FormatByteDecimalUnits(int64(m.Sys)),
		m.NumGC)
}

// k,v as arguments

type kvMap map[string]string

func (s kvMap) String() string {
	r := ""
	for k, v := range s {
		r += fmt.Sprintln(k, "=", v)
	}
	return r
}

// Set is called by the flag package each time the flag appears.
// It appends the new value to the slice.
func (s kvMap) Set(value string) error {
	kv := strings.Split(value, "=")
	if len(kv) != 2 {
		return fmt.Errorf("expected k=v entry for %s", value)
	}
	_, ok := s[kv[0]]
	if ok {
		return fmt.Errorf("duplicate key %s", kv[0])
	}
	s[kv[0]] = kv[1]
	return nil // Indicate success
}
