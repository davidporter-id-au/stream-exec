## stream-exec

#### Running commands

A convenient way to run a bash command each time for a set of data inputs. Takes a stream of JSON lines and executes a bash command, with the base json variables substituted in as environment variables.

eg: Given a file of json lines:

```json
{"user": "123", "document": 456}
{"user": "124", "document": 457}
```

Execute an arbitrary shell command with parallelism and retries:

```bash
cat records.json | stream-exec run --exec 'curl http://example.com/$user/$document/' --concurrency 10 --continue
```

In this example the json keys `user` and `document` are set as environment variables and are accessible by the subprocess.

Variables are passed in as envvars, rather than as simple string substitution, making it safe to use values containing spaces or special characters.

#### Monitoring and adjusting a running command

To find running processes:

```sh
$ stream-exec list
PID    RUNNING  DONE  FAILED  IN-FLIGHT  CONCURRENCY  EXEC
54858  23s      7     154     1          4            grep -qrO $word
```

To adjust the concurrency of a running instance:

```sh
$ stream-exec signal concurrency --pid 54858 --concurrency 5
concurrency set. scaling up/down may take a moment (pid 54858)
```

If only one stream-exec process is running, `--pid` can be omitted:

```sh
$ stream-exec signal concurrency --concurrency 5
```

To stop a process gracefully (drains in-flight work before exiting):

```sh
$ stream-exec signal stop 54858
sent stop signal to process 54858
```

#### What it's actually doing

Under the hood this is a simple invocation of `bash -c` with the envvars setup based on the input data. It's identical to a normal shell invocation and very simple.

#### Practical examples

Using bash for anything ends up running into quote conflicts very quickly, so for real-life examples, it's probably worth keeping the action you want as a script. 

For example, to get the weather for 100 cities from an API. We may wish to just use `curl`, but interpolating a large multiline command gets fiddly quickly.

given some cities: 
```json
{"city": "Berlin"}
{"city": "Albany"}
{"city": "New York"}
{"city": "Amsterdam"}
...
```

```shell
cat cities | stream-exec run -x 'curl https://goweather.xyz/v2/weather/$city' # this is missing lots of necessary headers, for example
```

To make this less messy, putting the actual request as a sub-script will make this tidier. Let's put it in `request.sh`. In this script, we can expect the envvars will be available as per usual ($city in this instance).

```shell
$ cat request.sh
curl 'https://goweather.xyz/v2/weather/$city' \
  --fail \
  -H 'accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7' \
  -H 'accept-language: en-AU,en-US;q=0.9,en-GB;q=0.8,en;q=0.7' \
  -H 'sec-ch-ua: "Chromium";v="146", "Not-A.Brand";v="24", "Google Chrome";v="146"' \
  -H 'user-agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36'
```

However, rather than looping in the bash script or trying to do concurrency-control there, stream exec will handle all of that, allowing the script author just to focus on the functionality of the request. 

```sh
# kick off the requests, keep it under 0.5 RPS so the API is happy and write out the data to output structured log
cat cities.json | stream-exec run -x './request.sh' --rps 0.5 --continue --output-log-path outputlog.json 
```

This will actually fail, since I forgot to handle the space in 'New York'. I can go find the failures in the output log

```sh
$ outputlog.json | jq '. | select(.Succeeded == false)'
{
  "Envvars": [
    "city=New York"
  ],
  "Params": {
    "ExecString": "./script.sh",
    "Retries": 0
  },
  "Stdout": "  % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current\n                                 Dload  Upload   Total   Spent    Left  Speed\n\r  0     0    0     0    0     0      0      0 --:--:-- --:--:-- --:--:--     0\r100   245    0   245    0     0    254      0 --:--:-- --:--:-- --:--:--   254\r100   245    0   245    0     0    254      0 --:--:-- --:--:-- --:--:--   254\n{\"temperature\":\"2 °C\",\"wind\":\"8 km/h\",\"description\":\"Clear\",\"forecast\":[{\"day\":\"Thursday\",\"temperature\":\"3 °C\",\"wind\":\"5 km/h\"},{\"day\":\"Friday\",\"temperature\":\"6 °C\",\"wind\":\"20 km/h\"},{\"day\":\"Saturday\",\"temperature\":\"4 °C\",\"wind\":\"21 km/h\"}]}  % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current\n                                 Dload  Upload   Total   Spent    Left  Speed\n\r  0     0    0     0    0     0      0      0 --:--:-- --:--:-- --:--:--     0curl: (6) Could not resolve host: York\n",
  "ExitCode": 6,
  "Succeeded": false
}
```

#### Justification

**Use-case**:
This is strictly for:
- safe, trusted inputs only. Absolutely no security guarantees on untrusted inputs
- JSON lines only, separated by newlines. No single long arrays either, break them up with `cat bigfile | jq ".[]" > jsonlines` first
- unix/linux/macOS environments, no Windows support
- For big or small files, it streams input so it doesn't load it fully into memory

**Comparison vs other tools**:

Why reinvent the perfectly-good `xargs -P`?
Because `xargs -I` doesn't allow for passing multiple parameters (unless I'm wildly mistaken — happy to be corrected) and you nearly always end up writing an identical bash script wrapper to pull out params with `jq`. Also logging and retry-handling are difficult with this.

Why not use a bash for-loop with some combination of `wait`?
Because it's bash — it's a bundle of sharp edges and nearly every time I write the script it's nearly identical. It's also very imperative and tedious.

Why not use gnu-parallel?
Because I don't *think* it's integrated with JSON lines. It's near-certainly more powerful, and in nearly every other use-case I'd suggest preferring it or xargs over this.

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
