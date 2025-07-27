# Auto Update Example

## Build

You can build the individual projects using `go build`.

### Client CLI

```
go build -ldflags "-X 'main.AvailablePokemon=pikachu,charmander,squirtle,bulbasaur'" -o ./pokemon ./pokemon
```

### Server

```
go build -o ./server ./server
```

## Format

To format all `.go` files:

```
gofmt -s -w .
```
