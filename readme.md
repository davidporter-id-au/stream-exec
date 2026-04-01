## stream-exec

#### Running commands

A convenient way to run a bash command each time for a set of data inputs. Takes a stream of JSON lines and executes a bash command, with the base json variables subsituted in as Envvars.

eg: Given a file of json lines:

```json
{"user": "123", "document": 456}
{"user": "124", "document": 457}
```

Execute an arbitrary shell command with parallelism and retries:

```bash
cat records.json | stream-exec run --x 'curl http://example.com/$user/$document/ --concurrency 10 --continue
```

In this example the json keys 'user' and 'document' have been set as environment variables 'user' and 'document' and are accessible by the subprocess.

Variables are passed in as envvars, rather than as simple string execution for exec, but ultimately it's an extremely simple wrapper around calling bash in a (a few) subshells. Hopefully as a means to make working with JSON inputs easier on the command line.

#### Monitoring and adjusting a running command

To find the running process:

```sh
$ stream-exec list
PID    RUNNING  DONE  FAILED  IN-FLIGHT  EXEC
54858  23s      7     154     1          grep -qrO $word
```

To set the concurrency of a running instance, use the `signal` subcommand

```sh
$ stream-exec signal concurrency --concurrency 5 --pid 54858  
concurrency set. scaling up/down make take a moment (pid 54858)
```

And to signal it to stop gracefully:
```sh
$ stream-exec signal stop 54858
sent stop signal to process 54858
```

#### Justification

**Use-case**:
This is strictly for:
- safe, trusted inputs only. Absolutely no security guarantees on untrusted inputs
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
macOS (Apple Silicon):
```
sudo curl -L https://github.com/davidporter-id-au/stream-exec/releases/latest/download/stream-exec_darwin_arm64 \
  -o /usr/local/bin/stream-exec && sudo chmod +x /usr/local/bin/stream-exec
```
Linux (amd64):
```
sudo curl -L https://github.com/davidporter-id-au/stream-exec/releases/latest/download/stream-exec_linux_amd64 \
  -o /usr/local/bin/stream-exec && sudo chmod +x /usr/local/bin/stream-exec
```
Linux (arm64):
```
sudo curl -L https://github.com/davidporter-id-au/stream-exec/releases/latest/download/stream-exec_linux_arm64 \
  -o /usr/local/bin/stream-exec && sudo chmod +x /usr/local/bin/stream-exec
```

#### Commands

**`stream-exec run`** — process a JSON lines stream

```
stream-exec run --exec <cmd> [options]

  --exec string          the command to run (required)
  --concurrency int      number of concurrent operations (default 1)
  --continue             continue on error
  --retries int          number of retry attempts on failure
  --dry-run              show what would run without executing
  --debug                enable debug logging
  --output-log-path      file to write stdout log (default: none)
  --err-log-path         file to write error log (default: auto-named .log)
```

**`stream-exec list`** — list running stream-exec processes

```
stream-exec list
```

Scans for active stream-exec processes and displays their PID, start time, and exec string.

**`stream-exec signal stop <pid>`** — stop a running process

```
stream-exec signal stop <pid>
```

Sends a graceful stop signal to the process with the given PID. In-flight commands are cancelled and the process exits cleanly.
