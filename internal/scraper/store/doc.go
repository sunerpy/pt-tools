// Package store provides GORM-based persistence for the pt-tools scraper subsystem.
//
// Models live here independently of pt-tools' top-level models/ package to avoid
// polluting the global schema and to allow the scraper to run standalone with its
// own SQLite file.
//
// Migration is driven by Migrate(db), tracked via its own scraper_schema_versions
// table (distinct from pt-tools' schema_versions table).
package store
