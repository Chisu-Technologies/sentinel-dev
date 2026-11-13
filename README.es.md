<!-- translated-from: README.md @ adeef98 on 2026-11-11 -->
[English](README.md) · Español

> Traducción de [README.md](README.md) hecha a partir del commit `adeef98`. Si difieren,
> prevalece el inglés. El resto de la documentación está en inglés.

[![ci](https://github.com/Chisu-Technologies/sentinel-dev/actions/workflows/ci.yml/badge.svg)](https://github.com/Chisu-Technologies/sentinel-dev/actions/workflows/ci.yml)
[![policy](https://github.com/Chisu-Technologies/sentinel-dev/actions/workflows/policy.yml/badge.svg)](https://github.com/Chisu-Technologies/sentinel-dev/actions/workflows/policy.yml)

# Sentinel

**Dale una tarea a un agente en tu propio servidor. Recibe un pull request que un QA
aparte probó antes de abrirlo.**

Sentinel ejecuta agentes de programación en una máquina que controlas tú. Cada tarea
corre en un sandbox que nunca tiene tus llaves. Un segundo agente, que no escribió el
cambio, lo prueba. El resultado es un pull request para que lo revises; el agente no
puede hacer merge.

> **Pre-alfa.** Hoy solo existen `sentinel up`, `sentinel down`, `sentinel cancel`,
> `sentinel doctor`, `sentinel run --dry-run`, `sentinel runs`, `sentinel verify`,
> `sentinel version` y `sentinel watch`. El resto de esta página es el diseño previsto;
> mira la [hoja de ruta](docs/roadmap.md).

## Cómo va a funcionar

```console
$ sentinel setup deploy@my-server     # instala el runtime en tu servidor, por ssh
$ sentinel run "fix the flaky login test"
$ sentinel watch                      # sigue la corrida desde cualquier terminal
$ sentinel pull                       # trae la rama del pull request
```

- **Tu portátil** ejecuta el comando `sentinel`, en Linux, macOS o Windows. Guarda un
  token revocable y nada más.
- **Tu servidor** hace el trabajo: Ubuntu 22.04, Ubuntu 24.04 o Debian 12, instalado
  con un comando por ssh. Las llaves del modelo y la credencial de GitHub se quedan
  ahí, fuera del sandbox. Una corrida sigue aunque cierres el portátil.
- **GitHub** recibe un pull request, que solo se abre después de que el QA apruebe.

Lee [cómo funciona](docs/how-it-works.md) y los [comandos](docs/commands.md).

## En qué se apoya

- **Un pull request comprobado.** Un agente de QA distinto del que escribió el cambio
  corre las pruebas y revisa el diff antes de que se abra el pull request.
- **Corridas que no se pierden.** Cada paso de una corrida se guarda al terminar, así
  que la corrida continúa después de que el servidor se reinicie. Reiniciar el
  servidor a media corrida es una de las pruebas que cierran la Fase 1; todavía no se
  ha ejecutado.
- **Reglas activas sin configurar nada.** Un límite de gasto por corrida, un sandbox
  endurecido y la prohibición de hacer merge. Una corrida que llega a su límite queda
  en pausa, y la reanudas con un límite mayor o con otro modelo.

## Prueba lo que ya existe

En Linux y macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/Chisu-Technologies/sentinel-dev/main/scripts/install.sh | sh
```

En Windows, en PowerShell:

```powershell
irm https://raw.githubusercontent.com/Chisu-Technologies/sentinel-dev/main/scripts/install.ps1 | iex
```

Qué hace el instalador, y cómo comprobar una descarga, está en
[cómo instalar el comando](docs/install.md). Con Go 1.26 o posterior también puedes
compilarlo: `go install github.com/Chisu-Technologies/sentinel-dev/cmd/sentinel@latest`.

Con Docker encendido, esto arranca Sentinel en tu máquina e imprime un enlace para
entrar a su panel:

```bash
sentinel up --email tu@ejemplo.com
```

```bash
sentinel doctor
```

```bash
sentinel doctor deploy@my-server
```

```bash
sentinel run --dry-run "fix the flaky login test"
```

```bash
sentinel verify 12
```

La primera forma comprueba git, la CLI de GitHub y ssh en tu máquina. La segunda
comprueba un servidor antes de instalar en él; solo lee, y no instala nada ahí. Cada
comprobación fallida dice cómo arreglarla. La tercera muestra lo que una tarea
enviaría desde el repositorio en el que estás, y no envía nada. La cuarta pide a un
servidor de Sentinel, el de `SENTINEL_URL`, que verifique el pull request 12 del
repositorio en el que estás, y muestra el veredicto; mira los
[comandos](docs/commands.md).

## Ediciones

| Edición | Para quién | Se paga con |
| --- | --- | --- |
| Abierta | Una persona o un equipo pequeño, en su propio servidor y con sus propias llaves de modelo | Nada; Apache-2.0 |
| Alojada | Quien no quiere operar un servidor | Una suscripción |
| Teams | Varios servidores, inicio de sesión único, auditoría, una bóveda central de llaves y límites por equipo | Una licencia anual |
| Enterprise | En tus instalaciones o en nube privada, también sin acceso a internet | Un contrato |

El QA, las corridas duraderas y la prohibición de hacer merge forman parte de la
edición abierta. Las ediciones de pago están previstas y no están en este repositorio.

## Qué sale de tu máquina

En la edición abierta, tu código y tus prompts van a tres sitios: tu servidor, GitHub
y el proveedor de modelos que elijas. El servicio de Sentinel recibirá un inicio de
sesión con GitHub y conteos de uso, como cuántas corridas hubo, cuánto duraron, qué
modelo usaron y cómo terminaron. No recibirá prompts, código ni diffs. La lista exacta
no es definitiva; estará en la [página de privacidad](docs/privacy.md) antes de la
primera versión, y lo que no esté en ella no se enviará.

Hoy `sentinel cancel`, `sentinel verify`, `sentinel runs` y `sentinel watch` son los
comandos que contactan con un servidor de Sentinel, el de `SENTINEL_URL`. Envían tu
llave de API, y el nombre del repositorio y el número del pull request (`verify`) o el
id de un run (`cancel`, `watch`); leen el estado de tus runs, y `watch` también sus
últimas líneas de log. Ninguno lee ni envía tu árbol de trabajo.

## Requisitos (previstos)

- En tu portátil: git, la CLI de GitHub (`gh`) y un cliente ssh.
- En tu servidor: Ubuntu 22.04, Ubuntu 24.04 o Debian 12, en x86_64 o arm64, con 4 GB
  de memoria y root o sudo sin contraseña.
- Una cuenta de GitHub y una llave del proveedor de modelos que elijas.

La cifra de memoria es el objetivo para el que se está dimensionando Sentinel, no un
mínimo medido todavía.

## Licencia y contribuciones

Apache-2.0; mira [LICENSE](LICENSE), [NOTICE](NOTICE) y
[THIRD_PARTY-NOTICES.md](THIRD_PARTY-NOTICES.md). Las vulnerabilidades se reportan en
privado como explica [SECURITY.md](SECURITY.md). Las contribuciones necesitan la firma
DCO; mira [CONTRIBUTING.md](CONTRIBUTING.md) y el
[código de conducta](CODE_OF_CONDUCT.md).

---

Sentinel no tiene relación con Microsoft Sentinel.
