<!-- translated-from: README.md @ 70560bd on 2026-11-07 -->
[English](README.md) · Español

> Traducción de [README.md](README.md) hecha a partir del commit `70560bd`. Si difieren,
> prevalece el inglés. El resto de la documentación está en inglés.

[![policy](https://github.com/Chisu-Technologies/sentinel-dev/actions/workflows/policy.yml/badge.svg)](https://github.com/Chisu-Technologies/sentinel-dev/actions/workflows/policy.yml)

# Sentinel para desarrolladores

**Tu agente escribe; Sentinel demuestra que funciona.**

Sentinel para desarrolladores comprueba el trabajo de tu agente de programación.
Ejecutará tus tests, arrancará tu aplicación y la recorrerá con un navegador real, y
después informará de lo que pasó de verdad, no de lo que el agente dice que pasó.

> **Pre-alfa.** Todavía no hay nada que instalar; este repositorio contiene el diseño
> planeado. La beta será sólo por invitación.

## Cómo funcionará

- **Un plugin** para Claude Code y, más adelante, para Codex. Quien piensa es tu
  agente, en tu propia sesión y con tu propia suscripción. Sentinel no te pedirá una
  API key.
- **Un programa local** que el plugin arranca como servidor MCP. Ejecutará tus tests,
  tu aplicación y un navegador en Docker, en tu máquina.
- **Una cuenta gratuita de Sentinel** que guardará el plan, la proyección del Brain (un
  mapa de los ficheros y símbolos de tu repositorio) y la evidencia.

Un día de trabajo: planear con `/sentinel:architect`, aprobar el plan en la web,
implementar y probar con `/sentinel:run` y, al final, revisar tú la rama local. Mira
los [comandos](docs/commands.md) y [cómo funciona](docs/how-it-works.md).

## Qué saldrá de tu máquina y qué no

Se envía a Sentinel:

- el nombre y el remoto del repositorio, sin el usuario ni el token que pueda llevar
  la URL;
- el plan y el estado de sus tareas;
- los informes del runner: comando, código de salida, duración, el final truncado de
  la salida y digests. Ese final es lo que impriman tus propios comandos, así que un
  test que falla o una traza de error pueden contener líneas de código. El filtrado de
  secretos es de mejor esfuerzo, no una garantía;
- una proyección del Brain: los nombres de ficheros y símbolos y cómo se relacionan,
  no su contenido, hasta 300 nodos y 64 KB.

Sentinel nunca sube tus ficheros de código ni tus diffs, y no lee tus ficheros de
entorno ni tus credenciales; lo que impriman tus comandos pasa por el filtro de mejor
esfuerzo de arriba. Las capturas de pantalla y los logs completos de la aplicación se
quedan en tu máquina salvo que actives la subida para ese repositorio.

Tu token de Sentinel vivirá en el llavero del sistema operativo. El programa nunca
leerá, guardará ni reenviará tu sesión de Claude o de ChatGPT, y los contenedores no
recibirán más secretos que las variables de prueba que declares para ese repositorio.

Los detalles, y lo que aún no está decidido, están en la
[página de privacidad](docs/privacy.md).

## La evidencia es reportada, no atestada

Tu máquina ejecuta las comprobaciones e informa de los resultados, así que en tu
máquina se puede falsificar la evidencia. Sentinel la etiqueta como "reportada", no
como "atestada". Mira el [modelo de seguridad](docs/security-model.md).

## Qué es abierto

El plugin, el programa local y los formatos publicados (plan, informe del runner, grafo
del Brain, evidencia) tendrán licencia MIT y estarán en este repositorio. El servicio
alojado, el motor de los planes de pago y los Heads son de código cerrado y no están
aquí.

## Requisitos (planeados)

- Docker.
- Claude Code en la terminal; la versión mínima se confirmará en el lanzamiento.
- Windows, macOS o Linux, en x64 o arm64. Ninguno está probado todavía, y aún no está
  decidido cómo se distribuirá el programa.
- Una cuenta de GitHub, para iniciar sesión.

## Hoja de ruta

Cuatro fases, en orden y sin fechas: la parte del servicio alojado, una beta cerrada en
Claude Code, QA con revisores y el Brain, y el lanzamiento público. Mira la
[hoja de ruta](docs/roadmap.md).

## Licencia y contribuciones

MIT; mira [LICENSE](LICENSE) y [THIRD_PARTY-NOTICES.md](THIRD_PARTY-NOTICES.md).
Informa de las vulnerabilidades en privado como explica [SECURITY.md](SECURITY.md).
Las contribuciones necesitan la firma del DCO; mira [CONTRIBUTING.md](CONTRIBUTING.md)
y el [código de conducta](CODE_OF_CONDUCT.md).

---

Claude Code es un producto de Anthropic y Codex uno de OpenAI; Sentinel no está
afiliado a ninguna de las dos. Sentinel para desarrolladores no tiene relación con
Microsoft Sentinel.
