"""Preserve defaults shipped by the installed game; override managed fields."""
import sys
import xml.etree.ElementTree as ET


def merge_config(template, overrides, destination):
    tree = ET.parse(template)
    root = tree.getroot()
    properties = {item.get("name"): item for item in root.findall("property")}
    for override in ET.parse(overrides).getroot().findall("property"):
        name = override.get("name")
        existing = properties.get(name)
        if existing is None:
            root.append(override)
        else:
            existing.set("value", override.get("value", ""))
    tree.write(destination, encoding="utf-8", xml_declaration=True)


if __name__ == "__main__":
    merge_config(*sys.argv[1:])
