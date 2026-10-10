package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
)

func getNetworkCardLinkInfo(bdf string) (string, string, error) {
	cmd := exec.Command("lspci", "-s", bdf, "-vvv")
	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("error executing lspci: %w\nOutput: %s", err, string(outputBytes))
	}
	output := string(outputBytes)

	// Regex to find Link Capabilities
	lnkCapRegex := regexp.MustCompile(`LnkCap:\s+Port\s+#\d+,\s+Speed\s+(\S+),\s+Width\s+(\S+)`)
	lnkCapMatch := lnkCapRegex.FindStringSubmatch(output)
	var supportedSpeed, supportedWidth string
	if len(lnkCapMatch) == 3 {
		supportedSpeed = lnkCapMatch[1]
		supportedWidth = lnkCapMatch[2]
	}
	fmt.Println("supported:", supportedSpeed, supportedWidth)
	// Regex to find Link Status
	lnkStaRegex := regexp.MustCompile(`LnkSta:\s+Speed\s+(\S+),\s+Width\s+(\S+)`)
	lnkStaMatch := lnkStaRegex.FindStringSubmatch(output)
	var currentSpeed, currentWidth string
	if len(lnkStaMatch) == 3 {
		currentSpeed = lnkStaMatch[1]
		currentWidth = lnkStaMatch[2]
	}

	if currentSpeed == "" || currentWidth == "" {
		return "", "", fmt.Errorf("link speed or width not found in lspci output")
	}

	return currentSpeed, currentWidth, nil
}

func main() {
	// Example usage: Replace with the actual BDF of your network card
	bdf := os.Args[1] // Example BDF
	speed, width, err := getNetworkCardLinkInfo(bdf)
	if err != nil {
		fmt.Println("Error:", err, "for ", bdf)
		return
	}
	fmt.Printf("Network card (%s) - Current Link Speed: %s, Width: %s\n", bdf, speed, width)
}
