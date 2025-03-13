package utils

// temporary notes
const Livebox = `
curl -s -X POST -H "Accept: */*" -H "Content-Type: application/x-sah-ws-4-call+json" -d '{"service":"sysbus.DeviceInfo", "method":"get", "parameters":{}}' http://192.168.10.1/ws | jq
`
