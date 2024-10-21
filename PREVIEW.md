# Preview version

You can test with a preview version of our app: https://dl.nspeed.app/nspeed-client/latest/

For Unix systems, you must set the execution flag to the binary: `chmod +x ./nspeed` for instance.

Builds are not planned or regulars nor advertised so check manually for update

`./nspeed -version all` will give full details of the used Go version and 3rd party packages.

## control commands:
2 new commands `then` and `from`:
- `then` allows to perform command in sequence rather then in parallel. For instance: `nspeed get url1 get url2` will download url1 and url2 at the same time. Whereas `nspeed get url1 then get url2` will download url1 then url2.
- 'from' allows to read command from a file or an url: `nspeed from https://dl.nspeed.app/cf` will download the url and read nspeed commands from it. each line

## Benchmarks:
the `bench` (`b` for short) is a shortcut to a loopback test :

`./nspeed b h1g` is the same has:

`./nspeed server -id s1 get -n 1 nspeed://s1/20g`

it setups a server named `s1` then `get`  20g of data from it.

`./nspeed b -h` to see all the predifined benchmarks.

You can execute multiple benchmarks at the same time by separating them with a coma (no space).

## TCP
NSpeed can now use TCP directly instead of HTTP. This can be changed by specific the protocol parameter (`-P protocol`) where protocol can be `tcp` (other accepted value is `http` which is the default).
nspeed server -P tcp -p 8888 -a ""

