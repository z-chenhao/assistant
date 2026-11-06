"""Keep system fixture expectations aligned with the forward migration catalog."""
import unittest
from pathlib import Path

from verify import STATE_SCHEMA_VERSION


class SchemaVersionTests(unittest.TestCase):
    def test_system_fixture_version_matches_contiguous_migration_catalog(self):
        root = Path(__file__).resolve().parents[2]
        versions = sorted(int(path.name.split('_', 1)[0]) for path in
                          (root / 'internal/agent/sqlite/migrations').glob('[0-9][0-9][0-9]_*.sql'))
        self.assertEqual(versions, list(range(1, STATE_SCHEMA_VERSION + 1)),
                         'Update the system fixture version when adding a forward migration')
