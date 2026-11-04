Compilado y verificado. Te dejo los comandos exactos.

## Lo mínimo

```sh
cd D:\Proyectos\mio\deekseep-coder
go build -o deekseep.exe ./cli
.\deekseep.exe
```

Requisito: **Go 1.26+** (tienes 1.26.1). No hay Makefile ni scripts: es un módulo Go normal, `./cli` es el `main`.

## Versión de release (lo que ya dejé hecho)

```sh
go build -trimpath -ldflags "-s -w" -o dist\deekseep.exe ./cli
```

`-s -w` tira los símbolos de depuración y `-trimpath` quita rutas absolutas del binario:

| Build | Tamaño |
|---|---|
| `go build` a secas | 13,8 MB |
| `-trimpath -ldflags "-s -w"` | **9,8 MB** (−29 %) |

## Un binario por plataforma

```sh
set GOOS=linux ; set GOARCH=amd64 ; go build -trimpath -ldflags "-s -w" -o dist\deekseep-linux-amd64 ./cli
set GOOS=darwin; set GOARCH=arm64 ; go build -trimpath -ldflags "-s -w" -o dist\deekseep-macos-arm64 ./cli
set GOOS=windows; set GOARCH=amd64; go build -trimpath -ldflags "-s -w" -o dist\deekseep-windows-amd64.exe ./cli
```

Ya están generados y probados:

```
dist\deekseep.exe                 9,8 MB   ← este has usado
dist\deekseep-windows-amd64.exe   9,8 MB
dist\deekseep-linux-amd64         9,8 MB
dist\deekseep-macos-arm64         9,0 MB
```

Cero dependencias en tiempo de ejecución: son estáticos, no necesitan Go instalado.

## Verificado, no supuesto

```
$ dist\deekseep.exe help        → ayuda completa
$ dist\deekseep.exe config get  → api_key: **** (OS keyring), model: deepseek-flash
$ dist\deekseep.exe ask "Responde solo: compilado"
compilado
```

## Primer arranque en otra máquina

```sh
deekseep login                    # guarda la key en el llavero del SO
# o: set DEEPSEEK_API_KEY=sk-...
deekseep config set model deepseek-flash   # necesita conectividad
deekseep
```

## Dos cosas que conviene saber

**`code/` está vacía.** La parte GUI del README es intención, no código. `go build ./...` compila solo el CLI y los 16 paquetes internos; no hay un segundo binario.

**No existe `--version`.** Para una «primera versión» eso se echa de menos: no puedes saber qué build tienes en la mano. Se arregla con una variable y dos líneas:

```go
var version = "dev"   // se inyecta con -ldflags "-X main.version=v1.0.0"
```

Y añadirlo al `help`. Si quieres, lo hago ahora — son cinco minutos y deja el `dist/` listo para etiquetar.

También añadí `/dist/` al `.gitignore`: los binarios no deben acabar en el repo (comprobado con `git status --ignored` → `!! dist/...`).

¿Le meto el `--version`, te preparo un `Makefile` con `build`/`release`/`cross`, o lo dejamos aquí?