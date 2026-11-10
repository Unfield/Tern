---
title: Working offline
date: 2026-11-08
tags: [pwa]
---
The todos app caches its assets in a service worker, so prerendered pages load without a network.

## How it works

The worker caches the build on install, then serves pages from the network first, with the cache as a fallback.
