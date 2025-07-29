# Auto Update Example

An example CLI tool with auto-update functionality written in `go`.

The example contains:

* A CLI tool that outputs greetings from various Pokemon:

    ```
    Ivysaur says, "Hi!".
    Wartortle says, "Hi!".
    Pikachu says, "Hi!".
    Squirtle says, "Hi!".
    Ivysaur says, "Hi!".
    Wartortle says, "Hi!".
    Squirtle says, "Hi!".
    ```

    The CLI tool will auto-update on startup, and if run in daemon mode, will
    check for updates on an interval. If an update is found, the tool will
    download it, stop the old version (child process), and start the new
    version (child process).
* A server that serves the versions of the CLI tool via HTTP(S). The server will 
    automatically find new versions of the CLI tool on the disk and make them
    available.

To build this demo, you'll need [`go` `1.24+`](https://go.dev/dl/).

### Server

To build:

```
go build -o ./server ./server
```

To run:

```
./server/server --settings server/server-properties.json
```

### Client CLI

To build the the CLI tool, you must specify the version, update URL, and names of the available pokemon.

For example:

```
VERSION=1.0.0; go build -ldflags "-X 'main.Version=$VERSION' -X 'main.UpdateUrl=http://localhost:8080' -X 'main.AvailablePokemon=pikachu,charmander,squirtle,bulbasaur'" -o ./pokemon/version/$VERSION/pokemon ./pokemon
```

```
VERSION=2.0.0; go build -ldflags "-X 'main.Version=$VERSION' -X 'main.UpdateUrl=http://localhost:8080' -X 'main.AvailablePokemon=pikachu,raichu,charmander,charmeleon,squirtle,wartortle,bulbasaur,ivysaur'" -o ./pokemon/version/$VERSION/pokemon ./pokemon
```

```
VERSION=3.0.0; go build -ldflags "-X 'main.Version=$VERSION' -X 'main.UpdateUrl=http://localhost:8080' -X 'main.AvailablePokemon=pikachu,raichu,charmander,charmeleon,charizard,squirtle,wartortle,blastoise,bulbasaur,ivysaur,venusaur'" -o ./pokemon/version/$VERSION/pokemon ./pokemon
```

To run:

```
./pokemon/pokemon -d
```

## Testing

Run the end-to-end tests:

```
go build -o ./e2e ./test && ./e2e
```

Or with docker:

```
docker run --rm --volume="$PWD:/auto-update-example/" golang:1.24-alpine sh -c "cd /auto-update-example/ && go build -o ./e2e ./test && ./e2e"
```

TODO Fix e2e.go to work correctly on docker. The manual test passes in docker,
and the network is correct so it's unclear what the problem is.

Or on Windows:

```
go build -o .\e2e.exe .\test && .\e2e.exe
```

TODO everything in the test seems to work (update, output etc), but the
`pokemon` CLI is returning exit code 1.

## Format

To format all `.go` files:

```
gofmt -s -w .
```

## Auto Update

To see the auto-update functionality in action:

1. Build 2 versions of the CLI:

    ```
    rm -r demo/ pokemon/version/
    VERSION=1.0.0; go build -ldflags "-X 'main.Version=$VERSION' -X 'main.UpdateUrl=http://localhost:8080' -X 'main.AvailablePokemon=pikachu,charmander,squirtle,bulbasaur'" -o ./pokemon/version/$VERSION/pokemon ./pokemon
    mkdir demo/ && cp ./pokemon/version/$VERSION/pokemon ./demo/pokemon
    VERSION=2.0.0; go build -ldflags "-X 'main.Version=$VERSION' -X 'main.UpdateUrl=http://localhost:8080' -X 'main.AvailablePokemon=pikachu,raichu,charmander,charmeleon,squirtle,wartortle,bulbasaur,ivysaur'" -o ./pokemon/version/$VERSION/pokemon ./pokemon
    ```

2. Build and start the server:

    ```
    go build -o ./server ./server &&
        ./server/server --settings server/server-properties.json
    ```

3. In another terminal, start the CLI:

    ```
    ./demo/pokemon --daemon
    ```

    The CLI should automatically update to version `2.0.0` at startup:

    ```
    Checking for updates...
    Successfully updated to 2.0.0
    Ivysaur says, "Hi!".
    Wartortle says, "Hi!".
    Pikachu says, "Hi!".
    Squirtle says, "Hi!".
    Ivysaur says, "Hi!".
    Wartortle says, "Hi!".
    Squirtle says, "Hi!".
    Checking for updates...
    ```

    You'll see v2.0.0 Pokemon greetings like Raichu, Wartortle, Ivysaur, and Charmeleon.

4. In another terminal, build version 3.0.0 of the CLI:

    ```
    VERSION=3.0.0; go build -ldflags "-X 'main.Version=$VERSION' -X 'main.UpdateUrl=http://localhost:8080' -X 'main.AvailablePokemon=pikachu,raichu,charmander,charmeleon,charizard,squirtle,wartortle,blastoise,bulbasaur,ivysaur,venusaur'" -o ./pokemon/version/$VERSION/pokemon ./pokemon
    ```

    The server should automatically begin serving the updated version and the
    CLI should automatically update to it:

    ```
    Wartortle says, "Hi!".
    Charmeleon says, "Hi!".
    Wartortle says, "Hi!".
    Squirtle says, "Hi!".
    Wartortle says, "Hi!".
    Checking for updates...
    Squirtle says, "Hi!".
    Successfully updated to 3.0.0
    Charizard says, "Hi!".
    Wartortle says, "Hi!".
    Bulbasaur says, "Hi!".
    Wartortle says, "Hi!".
    ```

    After the update, you'll see v3.0.0 Pokemon greetings like Blastoise, Venusaur, and Charizard.

## Alternatives

Another approach to this problem would be to simply send down a JSON blob of
all supported Pokemon. That would certainly be simpler, but I wanted
to showcase the ability to completely replace the full binary in a seamless or
near-seamless way.

The server could also support the ability to upload updates to it directly, but
that would require a full database and authentication scheme. The current
approach is simpler and could work from a build pipeline with SSH access to the
server. So SSH authentication and server filesystem permissions could be used in
lieu of server functionality.

The server could also have used gRPC rather than HTTP/JSON for communication, but
JSON/HTTP is easier to debug and works with a browser as well. It works well
for this simple case.
