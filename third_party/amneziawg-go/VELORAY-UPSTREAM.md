# VeloRay source pin

Upstream: https://github.com/amnezia-vpn/amneziawg-go
Commit: 457d920a1a7d103fc7354e0ca3979e493a681d5e
Go module: v0.2.20-0.20260724121833-457d920a1a7d
License: MIT; original LICENSE and copyright notices retained.

Complete source distributed in the pinned Go module. The only runtime source
change is in device/timers.go: the callback clears duration while holding
modifyingLock, together with its read, instead of writing after releasing the
lock. The upstream write races with Mod/Del and can erase a concurrently scheduled
pending state. A local replace directive makes builds and race tests use the fix.
No protocol constants, cryptography, packet format or upstream dependency versions
are changed. The root project tests include concurrent timer rescheduling,
encrypted TCP/UDP round trips, hot revocation and diagnostic clients. Source
manifests cover this directory; runtime packages retain the MIT license.
