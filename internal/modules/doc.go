// Package modules is the root namespace for business modules.
//
// A module receives its domain, application, transport and infrastructure
// directories only when its approved PRD is implemented. Keeping this package
// explicit prevents an untracked empty directory from masquerading as a module.
package modules
