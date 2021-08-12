# NSpeed API (draft)

The API is a minimalist "REST" API above HTTP. 
It can use a dedicated endpoint or extend a 'server' command. In both case, the url is always '/api/v1'.

The currently available API URIs are `HTTP GET` requests only and are:
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

### `/api/v1/help` and `/api/`
return this file
