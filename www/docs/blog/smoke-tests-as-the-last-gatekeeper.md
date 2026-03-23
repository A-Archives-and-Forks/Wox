---
title: Smoke Tests Before Release
description: How Wox runs a practical smoke test flow against the real app before changes ship.
date: 2026-03-20
---

# Smoke Tests Before Release

Wox has a Go backend, a Flutter desktop UI, plugin hosts, settings, storage, and platform-specific behavior. Once development speed picked up, the weak spot became obvious: it was too easy to make a change that looked fine in isolation but still broke the real app on startup.

So we added a smoke test flow that runs the actual startup path instead of checking pieces in isolation.

Under `wox.test/`, the runner starts a real `wox.core` process, points it at isolated data directories, waits for `/ping`, and then runs Flutter `integration_test` cases against that live backend. Every run gets its own artifact directory with logs, which makes failures easy to inspect later instead of trying to reproduce them from memory.

The runner only does a few things, but each of them matters. It rewrites `WOX_TEST_DATA_DIR` and `WOX_TEST_USER_DIR` so the test never pollutes a developer's normal setup. It prefers port `34987`, but falls back when the port is already taken. It does not assume the backend is ready based on timing. UI tests only start after `http://127.0.0.1:<port>/ping` is reachable. During smoke runs, telemetry stays off, and the flow records both `core.log` and `flutter_test.log`.

That is what turns it from a demo script into a usable gate. The value is not in how much logic the runner has. The value is in isolation, repeatability, and enough artifacts to debug a failure without guessing.

The first batch of cases lives in `wox.ui.flutter/wox/integration_test/launcher_smoke_test.dart`. It covers the paths that tend to break first and hurt most: opening the launcher, typing into the query box, moving through results with the keyboard, opening settings, switching between a few core settings pages, and reaching entries like themes, data, usage, and about. That is enough to answer a practical release question: does Wox still come up and behave like Wox?

We are not trying to turn smoke tests into a giant system. The point is to keep one reliable gate at the end of the flow. When changes come faster, especially with AI-assisted iteration, confidence has to come from a real run of the app, not from code that merely looks reasonable. For Wox, this smoke suite is that last check before we trust a change.
