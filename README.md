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
    download it, restart the process using the updated version, and shut down
    the original process.
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

## Format

To format all `.go` files:

```
gofmt -s -w .
```

## Auto Update

To see the auto-update functionality in action:

1. Build 2 versions of the CLI:

    ```
    VERSION=1.0.0; go build -ldflags "-X 'main.Version=$VERSION' -X 'main.UpdateUrl=http://localhost:8080' -X 'main.AvailablePokemon=pikachu,charmander,squirtle,bulbasaur'" -o ./pokemon/version/$VERSION/pokemon ./pokemon
    VERSION=2.0.0; go build -ldflags "-X 'main.Version=$VERSION' -X 'main.UpdateUrl=http://localhost:8080' -X 'main.AvailablePokemon=pikachu,raichu,charmander,charmeleon,squirtle,wartortle,bulbasaur,ivysaur'" -o ./pokemon/version/$VERSION/pokemon ./pokemon
    ```

2. Build and start the server:

    ```
    go build -o ./server ./server &&
        ./server/server --settings server/server-properties.json
    ```

3. Start the CLI:

    ```
    ./pokemon/pokemon --daemon
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

4. Build version 3.0.0 of the CLI:

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
