<!-- translated-from: README.md @ ba20e34 on 2026-11-06 -->
[English](README.md) · Español

> Esta es una traducción de [README.md](README.md) hecha a partir del commit `ba20e34`.
> Si difieren, prevalece el inglés. El resto de la documentación está en inglés.

[![policy](https://github.com/Chisu-Technologies/sentinel-dev/actions/workflows/policy.yml/badge.svg)](https://github.com/Chisu-Technologies/sentinel-dev/actions/workflows/policy.yml)

# Sentinel para desarrolladores

**Tu agente escribe; Sentinel demuestra que funciona.**

Sentinel para desarrolladores comprueba el trabajo de tu agente de programación.
Ejecutará tus tests, arrancará tu aplicación y la recorrerá con un navegador real, y
después informará de lo que pasó de verdad, no de lo que el agente dice que pasó. Los
resultados los informa tu propia máquina; mira
[qué es la evidencia y qué no](#qué-es-la-evidencia-y-qué-no).

Claude Code es un producto de Anthropic y Codex uno de OpenAI; Sentinel no está
afiliado a ninguna de las dos.

## Estado: pre-alfa

> **Todavía no hay nada que instalar.** Todo lo que se describe abajo está planeado.

Este repositorio es público desde el primer día para que veas qué se está construyendo
y con qué licencia. El acceso a la beta lo controlará Sentinel, no la visibilidad de
este repositorio. La beta es sólo por invitación. Por ahora las preguntas van por
[issues](https://github.com/Chisu-Technologies/sentinel-dev/issues/new/choose).

## Cómo funcionará

- Un plugin para Claude Code y, más adelante, para Codex. Trabajará con Claude Code y
  usará tu propia sesión; no la sustituirá.
- Un programa local que ejecutará tus tests, tu aplicación y un navegador en Docker,
  en tu propia máquina.
- Una cuenta gratuita de Sentinel que guardará el plan, la proyección del Brain y la
  evidencia. El Brain es el mapa que Sentinel hace de tu repositorio: qué ficheros y
  símbolos existen y cómo se relacionan entre sí.

Quien piensa es tu agente, con tu propia suscripción. Sentinel no pagará la inferencia
ni te pedirá una API key. Todo ocurrirá dentro de tu sesión interactiva, y los
resultados quedarán en ramas locales. Hacer push y abrir un pull request sigue siendo
decisión tuya.

> **Planeado (fase 1).** Un día de trabajo será así, sólo con los nombres de los
> comandos: planear con `/sentinel:architect`, aprobar el plan en la web con
> `/sentinel:approve`, implementar y probar con `/sentinel:run`, comprobar los cambios
> actuales con `/sentinel:verify` y, al final, revisar tú la rama. En la
> [página de comandos](docs/commands.md) (en inglés) está la fase de cada comando.

## Qué es abierto y qué no

> **Planeado.** Ninguna de las partes abiertas existe todavía; hoy este repositorio
> contiene la documentación y las comprobaciones que la protegen.

Serán abiertos, con licencia MIT y en este repositorio: el plugin (comandos,
subagentes y rúbricas de revisión), el programa local (login, servidor MCP, runner,
indexador del Brain y verificador de recibos) y los formatos publicados de los planes,
los informes del runner, el grafo del Brain y la evidencia.

No son abiertos: el servicio alojado, el motor de los planes de pago y los Heads. No
están en este repositorio. Mira [cómo funciona](docs/how-it-works.md) (en inglés).

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

Para el cuadro completo, incluido lo que aún no está decidido, lee la
[página de privacidad](docs/privacy.md) (en inglés).

## Qué es la evidencia y qué no

Los resultados los informa tu máquina al servicio alojado. En tu máquina se puede
falsificar la evidencia, así que Sentinel la etiqueta como "reportada" y nunca como
"atestada". Lee el [modelo de seguridad](docs/security-model.md) (en inglés) para ver
la intención del diseño y sus límites.

## Requisitos (planeados)

- Docker.
- Claude Code, en una versión mínima que se confirmará en el lanzamiento.
- Un sistema operativo compatible; mira la
  [página de compatibilidad](docs/compatibility.md) (en inglés).

## Instalación

Todavía no hay nada que instalar. El flujo planeado, con cada comando marcado como que
aún no funciona, está en [primeros pasos](docs/getting-started.md) (en inglés).

## Documentación

Toda en inglés por ahora.

- [Índice de la documentación](docs/README.md)
- [Primeros pasos](docs/getting-started.md) y [cómo funciona](docs/how-it-works.md)
- [Privacidad](docs/privacy.md) y [modelo de seguridad](docs/security-model.md)
- [Comandos](docs/commands.md) y [compatibilidad](docs/compatibility.md)
- [Preguntas frecuentes](docs/faq.md), [glosario](docs/glossary.md) y
  [hoja de ruta](docs/roadmap.md)

## Lo que viene

El trabajo está planeado en cuatro fases, en orden y sin fechas: la fase 0 construye la
parte del servicio alojado, la fase 1 es una beta cerrada en Claude Code, la fase 2
añade QA, revisores y el Brain, y la fase 3 es el lanzamiento público. Mira la
[hoja de ruta](docs/roadmap.md) (en inglés).

## Licencia

MIT. Mira [LICENSE](LICENSE). Los componentes de terceros están en
[THIRD_PARTY-NOTICES.md](THIRD_PARTY-NOTICES.md).

## Seguridad y contribuciones

Por favor, informa de las vulnerabilidades en privado; mira [SECURITY.md](SECURITY.md).
Las contribuciones son bienvenidas bajo el DCO; mira [CONTRIBUTING.md](CONTRIBUTING.md).

---

Los nombres de producto pertenecen a sus dueños. Sentinel para desarrolladores no tiene
relación con Microsoft Sentinel.
