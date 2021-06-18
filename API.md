# NSpeed API

The API is a minimalist REST API above HTTP. it can use a dedicated endpoint or extend a 'server' command. In both case, the url is always '/api'.

the API URI are:

`/api/add/`
add jobs, json format below.

`/api/list`
retrive the currently jobs status. json format below.

`/api/delete`
delete all jobs

`/api/run/&callback`
run all jobs

`/api/stop/`
stop all running jobs (use `list` to see current running status)

json format for a job:
    [
        {
            "command": string
            "flags": [{string, string}]
        }
    ]
