---
title: Smoke Tests as the Last Gatekeeper
description: How Wox built an automated smoke test flow to guard regressions and reliability.
date: 2026-03-20
---

# Smoke Tests as the Last Gatekeeper

When Wox moved faster, the risk profile changed with it.

Cross-platform desktop software already has enough moving parts on its own: a Go backend, a Flutter desktop UI, plugin hosts, settings, storage, and platform-specific behavior. Once AI starts helping us generate, refactor, and iterate code faster, delivery speed goes up again. That is good for productivity, but it also means regressions can sneak in through places that still look locally reasonable.

That is why we decided to build an automated smoke test flow for Wox. In the AI era, automated testing is no longer just an efficiency tool. It is the last gatekeeper before regressions reach users.

## Why We Needed It

Unit tests are useful, but they do not fully answer the question we care about most before shipping:

Can Wox actually start, connect, show the launcher, react to input, open settings, and survive the most important user paths without falling apart?

For Wox, many reliability failures happen at the boundaries:

- the Flutter UI talks to the real `wox.core` process
- ports, files, and user directories need to be set up correctly
- desktop windows must open and hide at the right time
- settings and navigation need to remain reachable after internal changes

These are exactly the kinds of issues that can slip through code review, especially when iteration gets faster.

## What We Built

We introduced a cross-platform end-to-end smoke test runner under `wox.test/`.

The current flow is intentionally simple and practical:

1. Start a real `wox.core` instance in development mode.
2. Force it to use isolated Wox data and user data directories.
3. Wait until the backend becomes reachable through `/ping`.
4. Run Flutter desktop `integration_test` cases against that live backend.
5. Save logs and test artifacts for later diagnosis.

This gives us a meaningful answer to a high-value question: does the real app still work in a real startup path?

## Why The Runner Looks Like This

The smoke runner in `wox.test/bin/run.dart` does a few important things that make it reliable enough to use as a gate:

- It creates a timestamped artifact directory for every run.
- It redirects Wox data and user data through `WOX_TEST_DATA_DIR` and `WOX_TEST_USER_DIR`, so the test never pollutes a developer's normal environment.
- It prefers the regular development port `34987`, but automatically falls back to a free local port when needed.
- It waits for `http://127.0.0.1:<port>/ping` before starting UI tests, instead of assuming startup timing.
- It disables telemetry during smoke runs.
- It captures both `core.log` and `flutter_test.log`.

This matters more than it may seem. A smoke test is only a gatekeeper if it is reproducible, isolated, and diagnosable after failure.

## What The Tests Cover Today

The first batch of smoke cases lives in `wox.ui.flutter/wox/integration_test/launcher_smoke_test.dart`.

They focus on the core paths that must keep working:

- launching the main window and verifying the launcher UI is visible
- confirming the query box can be shown and controlled
- validating keyboard navigation on search results
- checking that settings can be opened from the launcher
- verifying several settings pages can be reached and closed safely
- confirming theme, data, usage, and about related entry points remain accessible

These are not exhaustive tests, and they are not trying to be. The goal of a smoke suite is different: catch critical regressions early with a fast, stable, high-signal flow.

## Why This Matters More In The AI Era

AI makes it much cheaper to produce code. It also makes it cheaper to produce code that looks plausible.

That changes the job of engineering discipline.

The bottleneck is no longer only "can we write the code?" It is increasingly "can we trust the change?" When code generation, refactoring, and experimentation all speed up, the final quality gate has to become more concrete, not less.

For us, smoke tests are that concrete gate:

- they verify behavior instead of intent
- they cover integration boundaries where regressions actually happen
- they give the team a repeatable confidence check before merging or releasing
- they produce artifacts that make failures easier to debug instead of easier to ignore

Put differently, AI can help us move faster, but automated testing decides whether that speed is safe.

## What Comes Next

The current smoke flow targets the real backend plus the Flutter desktop integration path. That already gives us a useful baseline, but it is only the first layer.

The next natural step is to extend the same idea to packaged builds and broader reliability scenarios, while keeping the suite lightweight enough to stay useful in day-to-day development.

We do not want a test system that looks impressive but nobody trusts.

We want a gatekeeper that fails for the right reasons.

