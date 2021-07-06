# NSpeed API

The API is a minimalist REST API above HTTP. it can use a dedicated endpoint or extend a 'server' command. In both case, the url is always '/api'.

the API URI are:

### `/api/run/args`
run jobs , args = same as cmd line args (minus global flags)

### `/api/v1/headers`
return client headers

### `/api/v1/version`
return api & server version

### `/api/v1/ip`
return client ip address & port 

### `/api/v1/time`
return the server local time
