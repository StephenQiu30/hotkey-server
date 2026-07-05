package app

// routes.go is the central registry for new HTTP route wiring.
//
// As new capabilities are added (trending, hot-events), they register
// their route groups here. The corresponding platformhttp.Register*Routes
// function is called from the central NewRouter, and this file manages
// the app-level dependency provisioning for those routes.
//
// Currently all routes are registered in internal/platform/http/router.go's
// NewRouter(). New route groups should follow the same pattern:
//
//   1. Define handler + service in its own package
//   2. Add Register*Routes() in internal/platform/http/
//   3. Wire the service dependency in internal/app/routes.go
//   4. Add the Register*Routes() call in platformhttp.NewRouter()
