// SPDX-License-Identifier: Apache-2.0

// Package supervisor is ktw-gui's active-universe supervisor: the lifecycle
// of the universes the GUI has active (Master, ActiveCore), their per-universe
// paths, the live watches that refresh the frontend (admin, catalog, runs,
// workspace) and the terminal router. Universe CRUD is the hub-served
// OrchestratorService, reached as a pure KTSP client with its own types; the
// GUI writes no management state.
package supervisor
