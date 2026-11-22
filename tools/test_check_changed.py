"""Test quick-check selection from changed paths and existing checkout files."""
from contextlib import redirect_stderr, redirect_stdout
from io import StringIO
import unittest
from unittest.mock import patch

from check_changed import ROOT, go_test_packages, plan, run

# test_check_changed.py verifies language-specific checks selected by pre-push.
# Fixtures use existing source paths where related tests are expected, and missing
# paths for deletion handling. Process-launch mocks check argument preservation
# and failure boundaries without installing packages or connecting to application
# services, while command plans retain the same contract on Windows and POSIX.

# BASE supplies a recognizable revision for command assertions; the selector passes
# this value through to Git, formatting, and changed-package lint commands unchanged.
BASE = 'abc123'


def commands(files):
    """commands returns planned arguments with deterministic Go package discovery.

    The package callback echoes selected directories because these tests inspect
    command selection rather than invoke the local Go build-tag resolver.
    """
    return [args for _, args in plan(files, BASE, go_packages=lambda dirs: ['./' + d for d in dirs])]


def flat(files):
    """flat joins planned command arguments for readable selector assertions."""
    return [' '.join(args) for args in commands(files)]


class PlanTest(unittest.TestCase):
    """PlanTest verifies pre-push command selection across source and configuration changes."""

    def test_plugin_go_contract_change_checks_host_consumers(self):
        """An SDK contract change can break host code without editing backend files.

        The SDK and its host consumers must both be checked, while this Go-only
        path must not require a frontend package manager or browser build.
        """
        result = flat(['plugin-sdk/go/manifest/manifest.go'])
        self.assertIn('go test ./...', result)
        self.assertIn('go test ./internal/plugin/... ./internal/app', result)
        self.assertIn('bash ../../tools/golangci-lint.sh run ./...', result)
        self.assertFalse(any('pnpm' in line for line in result))

    def test_plugin_web_change_builds_sdk_and_consumer(self):
        """A frontend SDK change must compile the minimal consumer against its types.

        Frozen dependency installation precedes SDK tests, and the consumer build
        verifies the preset instead of relying only on declaration typechecking.
        """
        result = flat(['plugin-sdk/web/src/types.ts'])
        self.assertIn('make prepare-plugin-sdk-web', result)
        self.assertIn('pnpm --dir plugin-sdk/web test', result)
        self.assertFalse(any('examples/checkin' in line for line in result))
        self.assertFalse(any(line == 'go test ./...' for line in result))

    def test_release_tool_change_selects_isolated_regressions(self):
        """Release tooling changes need their isolated Git and registry regressions.

        The selected suite must run before a push without publishing a package
        or depending on credentials for a real release destination.
        """
        result = flat(['.agents/skills/tr-release/scripts/sdk_release.py'])
        self.assertIn("python3 -m unittest discover -s .agents/skills/tr-release/scripts -p test_*.py", result)

    def test_document_change_only_checks_whitespace(self):
        """A documentation-only change needs the shared whitespace check.

        No language-specific tool is selected when the changed path contains no
        executable source, build configuration, or locale data.
        """
        self.assertEqual(commands(['docs/index.md']), [['git', 'diff', '--check', BASE]])

    def test_go_change_lints_and_tests_changed_package(self):
        """A Go source change selects formatting, architecture, lint, and package tests.

        Both committed and working-tree formatting are checked, and architecture
        rescans external source inputs without cached results. Node and pnpm remain unnecessary.
        """
        result = flat(['backend/internal/apikey/service.go'])
        self.assertIn('python3 tools/format_go.py --check --base ' + BASE, result)
        self.assertIn('python3 tools/format_go.py --check', result)
        self.assertIn('env GOWORK=off go test -count=1 ./...', result)
        self.assertIn('bash ../tools/golangci-lint.sh run --build-tags=integration --new-from-rev='
                      + BASE + ' ./internal/apikey', result)
        self.assertIn('go test ./internal/apikey', result)
        self.assertFalse(any('pnpm' in line for line in result))

    def test_architecture_changes_rescan_backend_without_package_tests(self):
        """Architecture policy changes must recheck backend imports without a source edit.

        The scan bypasses cached results for policy, fixture, and module changes,
        while unrelated backend package tests and frontend checks remain unselected.
        """
        for file in ['tools/architecture/catalog.go', 'tools/architecture/policy_test.go',
                     'tools/architecture/go.mod']:
            with self.subTest(file=file):
                self.assertEqual(commands([file]), [
                    ['git', 'diff', '--check', BASE],
                    ['env', 'GOWORK=off', 'go', 'test', '-count=1', './...'],
                ])

    def test_go_dependency_change_checks_whole_module(self):
        """A Go dependency update can affect packages throughout the module.

        The selected tests cover every package and still avoid introducing any
        frontend dependency installation into the Go-only checks.
        """
        result = flat(['backend/go.sum'])
        self.assertIn('go test ./...', result)
        self.assertFalse(any('pnpm' in line for line in result))

    def test_deleted_package_skips_lint_and_test(self):
        """A deleted Go directory cannot be passed to package commands.

        Formatting and architecture checks remain selected, while CI checks the
        surviving packages that could still import the removed directory.
        """
        result = flat(['backend/internal/removed_package_for_test/gone.go'])
        self.assertFalse(any(line.startswith(('go test ./internal', 'bash ../tools')) for line in result))

    def test_frontend_source_change_runs_full_static_and_related_tests(self):
        """An existing frontend source change runs every frontend static check.

        Application tests remain limited to the related module graph, and the
        parser and formatter fixtures run through their explicit entry point.
        """
        result = flat(['frontend/src/main.ts'])
        self.assertIn('pnpm --dir frontend run lint:check', result)
        self.assertIn('pnpm --dir frontend run typecheck', result)
        self.assertIn('pnpm --dir frontend run test:tools', result)
        self.assertIn('pnpm --dir frontend exec vitest related --run --passWithNoTests src/main.ts', result)
        self.assertNotIn('pnpm --dir frontend run test:run', result)
        self.assertFalse(any('exec eslint' in line for line in result))

    def test_deleted_frontend_source_runs_full_suite(self):
        """A removed frontend module may disappear from the related-test graph.

        The full application suite checks remaining consumers, and static checks
        continue to cover the entire surviving frontend tree.
        """
        result = flat(['frontend/src/removed_for_test.ts'])
        self.assertIn('pnpm --dir frontend run lint:check', result)
        self.assertIn('pnpm --dir frontend run test:run', result)
        self.assertFalse(any('vitest related' in line for line in result))

    def test_frontend_dependency_change_installs_before_full_suite(self):
        """Frontend dependency files affect both package installation and every source consumer.

        Package, lockfile, and workspace changes each select a frozen install before
        static checks and the full application suite.
        """
        for file in ['frontend/package.json', 'frontend/pnpm-lock.yaml', 'frontend/pnpm-workspace.yaml']:
            with self.subTest(file=file):
                result = flat([file])
                install = 'pnpm --dir frontend install --frozen-lockfile'
                lint = 'pnpm --dir frontend run lint:check'
                self.assertLess(result.index(install), result.index(lint))
                self.assertIn('pnpm --dir frontend run test:run', result)

    def test_frontend_configuration_change_runs_full_suite_without_install(self):
        """A formatter or linter configuration change affects the frontend check policy.

        The full suite is selected, while package installation remains reserved
        for files that actually declare the dependency graph.
        """
        result = flat(['frontend/prettier.config.mjs'])
        self.assertIn('pnpm --dir frontend run lint:check', result)
        self.assertIn('pnpm --dir frontend run test:tools', result)
        self.assertIn('pnpm --dir frontend run test:run', result)
        self.assertFalse(any('install' in line for line in result))

    def test_backend_locale_change_runs_frontend_related_tests(self):
        """Backend locale content is imported by frontend locale consistency tests.

        A locale-only change retains related tests without adding unrelated
        frontend static checks or running the complete application suite.
        """
        result = flat(['backend/internal/pkg/locale/manifest.json'])
        self.assertIn('pnpm --dir frontend exec vitest related --run --passWithNoTests '
                      '../backend/internal/pkg/locale/manifest.json', result)
        self.assertFalse(any('lint:check' in line or 'typecheck' in line for line in result))

    def test_script_and_tool_changes_run_their_tests(self):
        """Deployment scripts and repository tools each have their own regression entry point.

        Changes to hooks and declaration-order code select tool tests even when
        no application source file changed in the push.
        """
        self.assertIn('make test-scripts', flat(['deploy/install.sh']))
        self.assertIn('make test-tools', flat(['tools/format_go.py']))
        self.assertIn('make test-tools', flat(['tools/declorder/reorder.go']))
        self.assertIn('make test-tools', flat(['.githooks/pre-push']))


class PackageTest(unittest.TestCase):
    """PackageTest checks package paths returned by native Go discovery."""

    def test_native_package_path_uses_slash_separators(self):
        """Go discovery returns a directory using the host's native separators.

        Package arguments must use forward slashes so the same package selection
        remains valid on Windows and POSIX hosts.
        """
        package_dir = str(ROOT / 'backend' / 'internal' / 'web')
        with patch('check_changed.subprocess.check_output', return_value=package_dir):
            self.assertEqual(go_test_packages(['internal/web']), ['./internal/web'])


class RunTest(unittest.TestCase):
    """RunTest verifies tool identity, argument boundaries, and command failures."""

    def test_resolved_executable_preserves_argument_boundaries(self):
        """A selected runtime can have spaces in its absolute path.

        Launching it must preserve the selected executable and each script or
        filename argument without interpreting them through another shell.
        """
        runtime = str(ROOT / 'selected tools' / 'bash.exe')
        args = ['bash', 'script with spaces.sh', 'literal $(value)']
        with patch('check_changed.shutil.which', return_value=runtime), \
                patch('check_changed.subprocess.call', return_value=0) as launch, \
                redirect_stdout(StringIO()):
            self.assertEqual(run([('.', args)]), 0)
        launch.assert_called_once_with([runtime, *args[1:]], cwd=ROOT)

    def test_missing_tool_reports_name_and_stops(self):
        """A missing tool must stop the check before creating any process.

        The exit status and diagnostic identify the missing executable so the
        caller can repair its tool environment.
        """
        errors = StringIO()
        with patch('check_changed.shutil.which', return_value=None), \
                patch('check_changed.subprocess.call') as launch, \
                redirect_stdout(StringIO()), redirect_stderr(errors):
            self.assertEqual(run([('.', ['missing-tool'])]), 127)
        launch.assert_not_called()
        self.assertIn('tools/check_changed.py: executable not found: missing-tool', errors.getvalue())

    def test_failed_command_prevents_later_checks(self):
        """A failed check must keep its original process exit status.

        Later checks remain unstarted so their output cannot hide the first
        failure or make an incomplete sequence appear successful.
        """
        with patch('check_changed.shutil.which', return_value='selected-runtime') as locate, \
                patch('check_changed.subprocess.call', return_value=23) as launch, \
                redirect_stdout(StringIO()), redirect_stderr(StringIO()):
            self.assertEqual(run([('.', ['first']), ('.', ['second'])]), 23)
        locate.assert_called_once_with('first')
        launch.assert_called_once_with(['selected-runtime'], cwd=ROOT)


if __name__ == '__main__':
    unittest.main()
