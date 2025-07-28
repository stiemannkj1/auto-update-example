# Auto Update Example

## Build

You can build the individual projects using `go build`.

### Server

```
go build -o ./server ./server
```

### Client CLI

Build the CLI specifying the names of the available pokemon:

```
VERSION=1.0.0; go build -ldflags "-X 'main.Version=$VERSION' -X 'main.UpdateUrl=http://localhost:8080' -X 'main.AvailablePokemon=pikachu,charmander,squirtle,bulbasaur'" -o ./pokemon/version/$VERSION/pokemon ./pokemon
```

```
VERSION=2.0.0; go build -ldflags "-X 'main.Version=$VERSION' -X 'main.UpdateUrl=http://localhost:8080' -X 'main.AvailablePokemon=pikachu,raichu,charmander,charmeleon,squirtle,wartortle,bulbasaur,ivysaur'" -o ./pokemon/version/$VERSION/pokemon ./pokemon
```

```
VERSION=3.0.0; go build -ldflags "-X 'main.Version=$VERSION' -X 'main.UpdateUrl=http://localhost:8080' -X 'main.AvailablePokemon=pikachu,raichu,charmander,charmeleon,charizard,squirtle,wartortle,blastoise,bulbasaur,ivysaur,venusaur'" -o ./pokemon/version/$VERSION/pokemon ./pokemon
```

## Format

To format all `.go` files:

```
gofmt -s -w .
```

## Run

### Server


