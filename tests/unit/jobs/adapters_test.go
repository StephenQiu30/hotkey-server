package jobs_test

// This file previously held compile-time interface satisfaction checks for
// old database.* repository types. Those types have been migrated to
// gormimpl.* implementations with updated method signatures (context.Context,
// model.* types). Interface compliance is now enforced by the repository
// interface definitions in internal/repository/.
//
// Remove these tests as the old types no longer exist. New compile-time
// checks for gormimpl types vs the existing jobs.* and scoring.* interfaces
// would fail because the method signatures differ — the gormimpl types
// implement the new repository interfaces, not the old package-specific ones.
