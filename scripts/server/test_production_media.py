"""Run the existing storage/snapshot regression cases against isolated prod hooks."""
import unittest
from unittest.mock import patch
from pathlib import Path
import production_media
import staging_media
import test_staging_media


class ProductionMediaTests(test_staging_media.MediaTests):
    def setUp(self):
        self.module = patch.object(test_staging_media, 'media', production_media)
        self.module.start()
        self.addCleanup(self.module.stop)

    def test_production_resources_are_separate(self):
        for name in ['MEDIA', 'MAINTENANCE', 'KEY', 'BACKUPS', 'UNIT', 'COMPOSE']:
            self.assertNotEqual(getattr(production_media, name), getattr(staging_media, name))
        self.assertEqual(production_media.MEDIA, Path('/var/lib/international-drift-hub/media'))
        self.assertEqual(production_media.UNIT, 'international-drift-hub.service')
        self.assertIn('/srv/international-drift-hub/ops/postgres.compose.yml', production_media.COMPOSE)


if __name__ == '__main__':
    unittest.main()
