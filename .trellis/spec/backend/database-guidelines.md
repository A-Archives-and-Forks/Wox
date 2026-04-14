# Database Guidelines

> Database patterns and conventions for this project.

---

## Overview

Wox uses SQLite as the primary local application database and accesses it through Gorm for core app data.

- Core bootstrap happens in `wox.core/database/database.go`.
- The shared app database file is `wox.db` under the user data directory.
- Schema bootstrapping for stable shared tables uses `db.AutoMigrate(...)`.
- Data and settings compatibility upgrades use the separate `migration/` package, not `AutoMigrate`.
- Some plugins intentionally use dedicated or external SQLite connections with `database/sql` when they own a separate file or read a third-party database. Example: Firefox bookmarks and clipboard history.

---

## Query Patterns

- Construct managers and stores with `*gorm.DB` and keep the DB handle on the struct. Examples: `setting.NewWoxSettingStore`, `setting.NewMRUManager`.
- Keep shared app DB models in `wox.core/database/` when multiple packages depend on them. Examples: `WoxSetting`, `PluginSetting`, `MRURecord`, `MigrationRecord`.
- Keep feature-local models near the feature when the schema is owned by that feature. Example: `plugin/system/shell/shell_history.go`.
- Wrap query failures with context using `fmt.Errorf(... %w)` unless the caller must branch on a sentinel such as `gorm.ErrRecordNotFound`.
- Use `db.Transaction(...)` for multi-step writes that must succeed or fail together. Example: `migration.RunWithDB`.

Examples:

- `wox.core/setting/mru.go`: branches on `gorm.ErrRecordNotFound`, creates or updates records, and wraps real failures with `%w`.
- `wox.core/setting/store.go`: serializes primitives directly and JSON-encodes complex values before saving through Gorm.
- `wox.core/plugin/system/shell/shell_history.go`: keeps feature-owned history schema near the shell plugin instead of forcing it into the shared `database` package.

---

## Migrations

There are two distinct migration paths:

1. Shared schema bootstrapping:
   `database.Init` calls `db.AutoMigrate(...)` for stable app-owned tables.
2. Compatibility/data migrations:
   `migration.Run` executes registered migrations in lexicographic ID order and records results in `migration_records`.

Rules for application-level migrations:

- Create one file per migration in `wox.core/migration/` using the `mYYYYMMDD_short_name.go` pattern.
- Register the migration in `init()` with `migration.Register(...)`.
- Implement `Up(ctx context.Context, tx *gorm.DB) error`.
- Use `ConditionalMigration` when the migration can be skipped safely.
- Use `PostCommitMigration` only for work that must run after the DB transaction commits.

Examples:

- `wox.core/migration/migrator.go`: loads applied records, sorts migrations, runs each migration in a transaction, and records `applied` or `skipped`.
- `wox.core/migration/m20251219_reset_theme.go`: a focused compatibility migration that updates one setting through a transaction-backed store.
- `wox.core/migration/README.md`: documents the intended authoring workflow and contract.

---

## Naming Conventions

- Rely on Gorm's default snake_case table and column naming unless there is a strong reason not to.
- Mark primary keys explicitly with Gorm tags, including composite keys. Examples: `PluginSetting`, `ToolbarMute`.
- Use singular Go struct names for persisted models: `MRURecord`, `MigrationRecord`, `Oplog`.
- Use integer Unix timestamps for operational records that are exchanged with other layers; use `time.Time` when Gorm lifecycle fields are useful. Example: `MRURecord` uses both `LastUsed` and `CreatedAt`/`UpdatedAt`.
- Store complex values as JSON strings when the field is opaque to SQL queries. Examples: `Icon`, `ContextData`, serialized plugin settings.

---

## Common Mistakes

- Do not use `AutoMigrate` for data repair or compatibility logic. Put that work in `migration/`.
- Do not open a new connection to the shared `wox.db` just to bypass `database.Init`; that skips the repo's DSN, PRAGMA, and integrity-check setup.
- Do not swallow `gorm.ErrRecordNotFound` as a generic failure when the code needs a create-or-update branch.
- Do not add schema that belongs only to one plugin into the shared `database` package unless other packages must also depend on it.
