\set ON_ERROR_STOP on

\getenv libraries_password LIBRARIES_DB_PASSWORD
\getenv reservations_password RESERVATIONS_DB_PASSWORD
\getenv ratings_password RATINGS_DB_PASSWORD

CREATE ROLE library_service LOGIN PASSWORD :'libraries_password';
CREATE ROLE reservation_service LOGIN PASSWORD :'reservations_password';
CREATE ROLE rating_service LOGIN PASSWORD :'ratings_password';

CREATE DATABASE libraries OWNER library_service;
CREATE DATABASE reservations OWNER reservation_service;
CREATE DATABASE ratings OWNER rating_service;

REVOKE ALL ON DATABASE libraries FROM PUBLIC;
REVOKE ALL ON DATABASE reservations FROM PUBLIC;
REVOKE ALL ON DATABASE ratings FROM PUBLIC;

\connect libraries
SET ROLE library_service;
\i /etc/postgresql/schemas/libraries.sql
RESET ROLE;

\connect reservations
SET ROLE reservation_service;
\i /etc/postgresql/schemas/reservations.sql
RESET ROLE;

\connect ratings
SET ROLE rating_service;
\i /etc/postgresql/schemas/ratings.sql
RESET ROLE;
