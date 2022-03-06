### JSON stream-exec

takes a stream of JSON lines and executes a bash command, with the base json variables subsituted in as Envvars.

This is just a convenience function for the typical use-case of looping over recorrds in some data-set, to perform some side-effect such as a network
request.

Given a file of json lines:

```json
{"user": "123", "document": 456}
{"user": "124", "document": 457}
```

Execute an arbitrary shell command with parallelism and retries:

```bash
cat records.json | stream-exec --exec 'curl -X POST http://example.com/$document -d "{\"user\": \"$user\" }"' --concurrency 10 --continue
```

In this example the json keys 'user' and 'document' have been set as environment variables and are accessible by the subprocess.

For JSON subkeys, strings, integers are assigned directly. JSON objects and arrays are JSON stringified and set to the JSON key.

#### Justification

**Use-case**:
This is strictly for:
- save inputs only. Absolutely no security guarantees on untrusted inputs
- JSON lines only, separated by newlines. No single long arrays either, break them up with `cat bigfile | jq ".[]" > jsonlines` first
- unix/linux environments, no windows support
- For big or small files, it streams input, so it doesn't load it fully into memory

**Comparision vs other tools**:

Why reinvent the perfectly-good `xargs -P`? 
Because, xargs -I doesn't allow for passing multiple parameters (unless I'm wildly mistaken - happy to be corrected) and you nearly always end up writing the nearly identical bash script wrapper for pulling out any params with `jq`. Also logging and retry-handling are diffult with this.

Why not use a bash for-loop with some combination of `wait`?
Because it's bash - it's a bundle of sharp edges and nearly every time I write the script it's nearly identical. It's also very imperative and tedious. 

Why not use gnu-parallel?
Because I don't *think* it's integrated with JSON lines. it's near-certainly more powerful, and in nearly every other use-case I'd suggest preferring it or xargs over this.

#### Installation
osx:
```
curl -L https://github.com/davidporter-id-au/stream-exec/releases/download/0.0.3/stream-exec_darwin \
  -o /usr/local/bin/stream-exec && chmod +x /usr/local/bin/stream-exec
```
linux:
```
curl -L https://github.com/davidporter-id-au/stream-exec/releases/download/0.0.3/stream-exec_linux \
  -o /usr/local/bin/stream-exec && chmod +x /usr/local/bin/stream-exec
```
