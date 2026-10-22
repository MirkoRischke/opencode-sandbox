import unittest

import quality


class QualityReportTests(unittest.TestCase):
    def test_methods_with_identical_names_remain_separate(self):
        issues = []
        for receiver in ("a *First", "Second"):
            issues.append({
                "FromLinter": "gocognit",
                "Text": "cognitive complexity 3 of func `Close` is high (> -1)",
                "Pos": {"Filename": str(quality.ROOT / "example.go"), "Line": 12},
                "SourceLines": [f"func ({receiver}) Close() error {{"],
            })
        rows = quality.parse_issues(issues)
        self.assertEqual(set(rows), {"example.go:(*First).Close", "example.go:(Second).Close"})

    def test_line_moves_are_not_reported_as_metric_changes(self):
        row = {"file": "example.go", "function": "run", "line": 1,
               "cognitive": 4, "cyclomatic": 3}
        baseline = {"functions": {"example.go:run": row}}
        current = {"functions": {"example.go:run": dict(row, line=20)}}
        changes = quality.render(current, baseline).split("## Changes from baseline")[1]
        self.assertNotIn("example.go:run", changes)

    def test_extracted_helpers_and_removed_functions_are_visible(self):
        old = {"file": "example.go", "function": "old", "line": 1,
               "cognitive": 20, "cyclomatic": 12}
        new = dict(old, function="new", cognitive=5, cyclomatic=4)
        report = quality.render({"functions": {"example.go:new": new}},
                                {"functions": {"example.go:old": old}})
        self.assertIn("| `example.go:new` | — → 5 |", report)
        self.assertIn("| `example.go:old` | 20 → — |", report)

    def test_tests_have_their_own_thresholds(self):
        row = {"file": "example.go", "cognitive": 12, "cyclomatic": 4, "long_lines": 70}
        self.assertEqual(quality.hotspots(row), [])
        self.assertEqual(quality.hotspots(dict(row, file="example_test.go")),
                         ["cognitive=12>10", "long_lines=70>60"])

    def test_unknown_diagnostics_fail_instead_of_silently_losing_metrics(self):
        with self.assertRaisesRegex(ValueError, "Unexpected diagnostic"):
            quality.parse_issues([{"FromLinter": "typecheck", "Text": "does not compile"}])


if __name__ == "__main__":
    unittest.main()
