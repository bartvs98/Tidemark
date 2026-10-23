# Verify scenarios

`node scripts/dev/verify.mjs --scenario scripts/dev/scenarios/<name>.json` seeds states the plain snapshot can't show (rules, reviews, image changes, open menus, other tabs) and takes named shots. Themes, widths and browsers come from the usual flags. Add `--pages` to shoot full pages too, or `--serve` to keep the seeded server running.

Committed scenarios use the made-up demo host (`"base": "scripts/dev/demo-snapshot.json"`), never a real host's snapshot. `--snapshot` overrides `base` for a local run.

```jsonc
{
  "base": "scripts/dev/demo-snapshot.json",   // snapshot to start from (optional)
  "config": "containers:\n  nginx:\n    pin: \"1.31\"\n",   // written to the data dir as config.yaml (optional)
  "snapshot": [                                // patched before the server starts (optional)
    { "container": "nginx", "labels": { "tidemark.pin": "1.31", "some.label": null } },  // null removes a label
    { "container": "traefik", "newImage": true }                // same image under a new ID: a pull + recreate
  ],
  "steps": [                                   // setup steps run in order after the startup check
    { "api": "PUT /api/containers/grafana/review", "body": { "kind": "reviewed", "version": "{latest:grafana}" } },
    { "snapshot": [{ "container": "traefik", "newImage": true }] },   // takes effect at the next check
    { "check": true },                                                 // run a check and wait for it
    { "sql": "scripts/dev/scenarios/stats.sql" },                      // run a SQL file (path from the repo root) against the data dir's tidemark.db, e.g. to seed history
    // shots are taken after all setup steps, per browser × theme × width: <browser>-<shot>-<theme>-<width>.png
    { "shot": "history", "path": "/containers/traefik",
      "view": "list",                          // dashboard view (list | tiles), for path "/"
      "do": [{ "click": "[role=tab]", "text": "History" }, { "key": "Escape" }, { "wait": 500 }],   // "type": "text" types into the focused element
      "element": "main",                       // crop to this element (otherwise the full page)
      "full": false }                          // viewport only instead of the full page
  ]
}
```

Placeholders in step bodies: `{latest:<container>}` is that container's latest version at that moment, `{in:7d}` / `{in:12h}` an ISO time from now. A click target that isn't found, or a missing `element`, is reported as a `scenario` problem.

`sql` steps run with `node:sqlite` while the server is up, so they see the schema the migrations made. Use SQLite date functions relative to `'now'` so seeded history stays recent; put the step after the startup check (before `check`, if the next check should see it). `stats.json` + `stats.sql` seed about 60 days of check runs, run stats, update history and AI calls for the stats page.
