### JSON stream-exec

takes a stream of JSON lines and executes a bash command, with the base json
variables subsituted in as Envvars.

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

#### Getting started
osx:
```
curl -L https://github.com/davidporter-id-au/stream-exec/releases/download/0.0.0/stream-exec_darwin \
  -o /usr/local/bin/stream-exec && chmod +x /usr/local/bin/stream-exec
```
linux:
```
curl -L https://github.com/davidporter-id-au/stream-exec/releases/download/0.0.0/stream-exec_linux \
  -o /usr/local/bin/stream-exec && chmod +x /usr/local/bin/stream-exec
```
