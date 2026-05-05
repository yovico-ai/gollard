# gollard

A Go port of [Andrew Rademacher's **mallard**](https://github.com/AndrewRademacher/mallard) —
no-frills SQL migrations for PostgreSQL.

## Why mallard / gollard

Most migration tools optimize for a single deploy target: you have one
production database, one staging database, and the migration history lives
beside it. Mallard is built for a different problem — **keeping a schema
consistent across many independent databases owned by different developers on
a distributed team.**

Each developer has their own local database. Schema changes arrive through
Git, not through a central migration runner. Mallard handles that case
unusually well:

- **Dependency resolution.** Migrations declare their `requires:` and the tool
  topologically orders them. Two teammates can add unrelated migrations on
  separate branches and merge without renaming files or fighting over a linear
  sequence number.
- **Content-addressed checksums.** Every applied migration's SHA-256 is
  recorded. If someone edits a migration that's already been applied
  somewhere, the next run refuses to proceed instead of silently diverging
  schemas across machines.
- **No down migrations.** Schemas are accretive. There's nothing to roll back
  inconsistently — you write a new forward migration that fixes the issue.
- **SQL-native files.** Migration metadata lives inside SQL comments, so the
  files are still valid SQL you can run by hand, paste into psql, or feed to
  any other tool. No YAML/JSON sidecars.

That combination — dependency graph + checksum guard + accretive forward-only
+ plain SQL — is rare. Most tools give you one or two of these. Mallard gives
you all four, which is what makes it a good fit for distributed teams where
nobody is the authoritative source of database truth.

`gollard` is a faithful port: same migration file format, same CLI surface,
same internal `mallard` schema in PostgreSQL. Existing mallard-managed
databases work unchanged.

## Install

```sh
go install github.com/yovico/gollard/cmd/gollard@latest
```

Or build from source:

```sh
cd go
go build ./cmd/gollard
```

A Debian package can be built via `scripts/deb-package.sh` (local, requires
`fpm`) or `docker/scripts/build-deb` followed by `docker/scripts/extract-deb`.

## Usage

```
gollard COMMAND [OPTIONS]

Commands:
  migrate ROOT             Apply unapplied migrations under ROOT.
  confirm-checksums ROOT   Compare applied checksums against the directory.
  repair-checksum ROOT NAME  Replace the DB checksum with the directory's.
  version                  Print version.

Connection flags (shared by migrate / confirm-checksums / repair-checksum):
  --host ARG       Server host (default 127.0.0.1)
  --port ARG       Server port (default 5432)
  --user ARG       Username     (default postgres)
  --password ARG   Password     (default empty)
  --database ARG   Database name (required)

Migrate-only:
  -t, --test       Run tests after migration.
```

Example:

```sh
gollard migrate ./sql --host db.local --user me --password s3cret --database app
```

## Migration file format

Migrations live in plain `.sql` files. Each migration is preceded by a
metadata block written in SQL comments:

```sql
-- #!migration
-- name: "tables/phone",
-- description: "Phone numbers attached to a person.",
-- requires: ["tables/person"];
SET search_path TO contact;

CREATE TABLE phone(
    id        BIGSERIAL NOT NULL,
    owner_id  bigint    NOT NULL,
    digits    text      NOT NULL,
    PRIMARY KEY (id),
    FOREIGN KEY (owner_id) REFERENCES person(id)
);
```

Multiple migrations can live in one file. Tests use the same syntax with
`#!test` and run inside a transaction that is always rolled back:

```sql
-- #!test
-- name: "samplePerson",
-- description: "Insert a whole person.";
INSERT INTO person (name_first, name_last) VALUES ('John', 'Doe');
```

A complete example tree lives under [`sql/example-contacts/`](./sql/example-contacts).

## Layout

```
cmd/gollard/         CLI entry point
gollard/             Library: parser, graph, postgres, commands
sql/mallard/         Bootstrap schema embedded into the binary
sql/example-contacts/  Example migration tree used by the integration test
docker/              Dockerfile + scripts to build a .deb
vagrant/             Provisioning script for a Vagrant dev VM
scripts/             Local build helpers
```

## Development

```sh
go test ./...                    # parser tests; skips integration test if no PG
go test ./gollard -run TestContactDeploy -v   # full integration test
```

The integration test expects a local PostgreSQL with a `vagrant`/`password`
superuser (matching `vagrant up`'s provisioning). Override via the standard
`PGHOST` / `PGUSER` / `PGPASSWORD` environment variables.

## License

MIT, inherited from upstream mallard. See [LICENSE](./LICENSE).
