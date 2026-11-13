#!/bin/bash
# ============================================================================
# Init de Postgres del compose UNIFICADO: UN servidor, TRES bases de datos.
#   - sentinel : la plataforma SaaS (platform/db.py, vault de credenciales).
#   - temporal : la persistencia principal del server de Temporal.
#   - temporal_visibility : el indice de visibilidad de Temporal; usa otra
#     base porque ambos esquemas mantienen su propia version.
#
# Se ejecuta SOLO la primera vez (volumen vacío), por el entrypoint oficial
# de la imagen postgres (todo *.sh / *.sql en /docker-entrypoint-initdb.d/).
# El usuario y la DB principal (sentinel) los crea la imagen con POSTGRES_USER
# / POSTGRES_DB; aquí añadimos las dos bases de Temporal (mismo rol/owner).
#
# Decisión (ver deploy/README.md): UN Postgres con tres DBs en vez de varias
# instancias. Menos RAM, un volumen, un healthcheck; el aislamiento que
# importa ahora es la frontera de VM por tenant. Backups/restore/HA se resuelven
# en la capa de VM/proveedor o con tooling propio, no moviendo datos a un
# servicio compartido entre tenants.
# ============================================================================
set -euo pipefail

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-SQL
    SELECT 'CREATE DATABASE temporal'
    WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'temporal')\gexec
    SELECT 'CREATE DATABASE temporal_visibility'
    WHERE NOT EXISTS (
        SELECT FROM pg_database WHERE datname = 'temporal_visibility'
    )\gexec
SQL

echo "[init] bases 'temporal' y 'temporal_visibility' aseguradas (owner: $POSTGRES_USER)"
