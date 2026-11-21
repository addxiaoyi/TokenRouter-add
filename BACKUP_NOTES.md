# Backup: src-compat-20261119

Server trees backed up on 2026-11-21 before deletion from the relay server
(/opt/tokenrouter/src-compat-20261119).

- Base commit: 10f8e82266359d92f24f99705ed687cba03640bd (this repo)
- This branch holds every file that differed from the base commit:
  - 487 files from the main compat tree (TokenRouter-add-10f8e82...) at their
    normal repository paths.
  - 263 files from the sibling backend-incomplete-fork tree, stored under
    backend-incomplete-fork/ (paths relative to that tree's root, which
    mirrors backend/ layout: cmd/, internal/, ent/, migrations/).
- Deleted vs base in the compat tree (21 paths, cannot be removed by the
  snapshot push, recorded here):
  - backend/internal/app/ops_monitor.go
  - backend/internal/ops/httpapi/doc.go
  - backend/internal/ops/httpapi/monitor.go
  - backend/internal/ops/httpapi/monitor_routes.go
  - backend/internal/ops/httpapi/monitor_test.go
  - backend/internal/ops/monitor (directory)
  - backend/internal/ops/postgres/monitor.go
  - backend/internal/ops/postgres/monitor_aggregation.go
  - backend/internal/ops/postgres/monitor_integration_test.go
  - backend/internal/partner (directory)
  - backend/internal/plugin/data.go
  - backend/internal/plugin/data_test.go
  - backend/internal/plugin/httpapi/data.go
  - backend/internal/plugin/httpapi/data_test.go
  - backend/internal/plugin/provider/data_archives.go
  - backend/internal/plugin/provider/data_archives_test.go
  - backend/internal/plugin/provider/data_files.go
  - backend/internal/plugin/provider/data_files_test.go
  - backend/internal/routing/postgres/group_availability.go
  - backend/internal/routing/postgres/group_availability_integration_test.go
  - backend/migrations/313_passive_channel_monitor.sql
- Not backed up: the compiled binary tokenrouter-compat at the compat tree
  root — a build output of this source.

Reconstruct: check out the base commit, overlay the files on this branch,
and remove the deleted paths listed above.
