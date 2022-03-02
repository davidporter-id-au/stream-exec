### stream-exec

takes a stream of JSON lines and executes a bash command, with the base json
variables subsituted in as Envvars.

This is just a convenience function for the typical use-case of looping over
recorrds in some data-set, to perform some side-effect such as a network
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
