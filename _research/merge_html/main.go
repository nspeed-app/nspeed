package main

import (
	"fmt"
	"log"

	"nspeed.app/utils"
)

const src = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>NSpeed Webview</title>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/jquery/3.6.3/jquery.min.js"></script>
    <script src="webview/statics/nspeed.js"></script>
</head>

<body>
HELLO
</body>
</html>
`

func main() {
	s, err := utils.InlineScript(src)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(s)
}
