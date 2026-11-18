import tempfile
import unittest
from pathlib import Path
import xml.etree.ElementTree as ET

from merge_config import merge_config


class ConfigMergeTests(unittest.TestCase):
    def test_preserves_shipped_defaults_and_replaces_managed_values(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "template.xml").write_text('<ServerSettings><property name="FutureSetting" value="keep"/><property name="ServerName" value="default"/></ServerSettings>')
            (root / "managed.xml").write_text('<ServerSettings><property name="ServerName" value="A &amp; B"/><property name="SandboxCode" value="ABC"/></ServerSettings>')
            merge_config(root / "template.xml", root / "managed.xml", root / "result.xml")
            values = {item.get("name"): item.get("value") for item in ET.parse(root / "result.xml").getroot()}
            self.assertEqual(values, {"FutureSetting": "keep", "ServerName": "A & B", "SandboxCode": "ABC"})
            self.assertIn('value="default"', (root / "template.xml").read_text())


if __name__ == "__main__":
    unittest.main()
