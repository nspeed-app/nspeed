# NSpeed API (draft v 0.10)

The API is a minimalist "REST" API above HTTP. 
It can use a dedicated http server or extend a 'server' command. In both case, the path in the url always starts with `/api/v1/..`.

The currently available API endpoints are only with the `GET` verb and are:
### `/api/v1/help` and `/api/`
return this file
### `/api/v1/run/command/args...`
run job(s) , same as invoking `npeed` from the command line , pass arguments in the `args` query parameter.

example:  
`/api/v1/run/get?args=get%20http://google.com`  
is equivalent to   
`nspeed get http://google.com`  

### `/api/v1/headers`
return client headers

### `/api/v1/version`
return api & server version

### `/api/v1/ip`
return client ip address & port 

### `/api/v1/time`
return the server local time in human readable format
### `/api/v1/time/unix` 
return the server local time in Unix time (= the number of seconds elapsed since January 1, 1970 UTC)

### `/api/v1/stats/info[/field1[/field2[/...]]`
return some os/hardware informations about the host. either all infos or a selection of named fields.
for instances:  
*  `api/v1/stats/info` return all informations.  
* `api/v1/stats/info/os` return the OS  
*  `api/v1/stats/info/os/platform` return the OS and the platform  

### `/api/v1/stats/mem[/gc]` 
return some memory informations about the host. Optionnally force a Go Garbage Collection by appending `gc`

