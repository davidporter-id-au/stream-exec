

Given a file of json lines:

```json
{"user": "123", "document": 456}
{"user": "124", "document": 457}
```

Execute an arbitrary shell command with parallelism and retries:

```bash
cat records.json | stream-exec --exec 'curl -X POST http://example.com/$document -d "{\"user\": \"$user\" }"' --concurrency 10 --retry-on-error --stdout-log-output output.log 
```
