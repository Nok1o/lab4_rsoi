#!/usr/bin/env bash

set -euo pipefail

variant="${VARIANT:-v4}"
init_directory="${POSTGRES_INITDB_DIR:-/docker-entrypoint-initdb.d}"

psql \
  --username "${POSTGRES_USER:-postgres}" \
  --dbname "${POSTGRES_DB:-postgres}" \
  --set ON_ERROR_STOP=1 \
  --file "$init_directory/scripts/db-$variant.sql"

if [[ "$variant" == "v4" ]]; then
  PGPASSWORD="${RESERVATIONS_DB_PASSWORD:-test}" psql \
    --username "${RESERVATIONS_DB_USER:-program}" \
    --dbname reservations \
    --set ON_ERROR_STOP=1 \
    --file "$init_directory/scripts/schema-reservation.sql"

  PGPASSWORD="${LIBRARIES_DB_PASSWORD:-test}" psql \
    --username "${LIBRARIES_DB_USER:-program}" \
    --dbname libraries \
    --set ON_ERROR_STOP=1 \
    --file "$init_directory/scripts/schema-library.sql"

  PGPASSWORD="${RATINGS_DB_PASSWORD:-test}" psql \
    --username "${RATINGS_DB_USER:-program}" \
    --dbname ratings \
    --set ON_ERROR_STOP=1 \
    --file "$init_directory/scripts/schema-rating.sql"
fi
