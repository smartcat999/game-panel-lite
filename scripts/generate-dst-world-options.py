#!/usr/bin/env python3
"""Generate the DST provider world-option manifest from a current scripts.zip.

Usage:
  scripts/generate-dst-world-options.py /path/to/scripts.zip \
    apps/api/internal/provider/dst/dst_world_options.json --build 740477
"""

from __future__ import annotations

import argparse
import json
import re
import zipfile
from pathlib import Path


DESCRIPTION_VALUES = {
    "frequency_descriptions": ["never", "rare", "default", "often", "always"],
    "worldgen_frequency_descriptions": ["never", "rare", "uncommon", "default", "often", "mostly", "always", "insane"],
    "ocean_worldgen_frequency_descriptions": ["ocean_never", "ocean_rare", "ocean_uncommon", "ocean_default", "ocean_often", "ocean_mostly", "ocean_always", "ocean_insane"],
    "starting_swaps_descriptions": ["classic", "default", "highly random"],
    "petrification_descriptions": ["none", "few", "default", "many", "max"],
    "speed_descriptions": ["never", "veryslow", "slow", "default", "fast", "veryfast"],
    "disease_descriptions": ["none", "random", "long", "default", "short"],
    "day_descriptions": ["default", "longday", "longdusk", "longnight", "noday", "nodusk", "nonight", "onlyday", "onlydusk", "onlynight"],
    "season_length_descriptions": ["noseason", "veryshortseason", "shortseason", "default", "longseason", "verylongseason", "random"],
    "season_start_descriptions": ["default", "winter", "spring", "summer", "autumn|spring", "winter|summer", "autumn|winter|spring|summer"],
    "size_descriptions": ["small", "medium", "default", "huge"],
    "branching_descriptions": ["never", "least", "default", "most", "random"],
    "loop_descriptions": ["never", "default", "always"],
    "loop_plus_descriptions": ["never", "rare", "default", "often", "always"],
    "complexity_descriptions": ["verysimple", "simple", "default", "complex", "verycomplex"],
    "specialevent_descriptions": ["none", "default"],
    "extraevent_descriptions": ["default", "enabled"],
    "extrastartingitems_descriptions": ["0", "5", "default", "15", "20", "none"],
    "atrium_descriptions": ["veryslow", "slow", "default", "fast", "veryfast"],
    "autodetect": ["never", "default", "always"],
    "yesno_descriptions": ["never", "default"],
    "dropeverythingondespawn_descriptions": ["default", "always"],
    "spawnmode_descriptions": ["fixed", "scatter"],
    "enableddisabled_descriptions": ["none", "always"],
    "ghostenabled_descriptions": ["none", "always"],
    "resetime_descriptions": ["none", "slow", "default", "fast", "always"],
    "nonlethal_descriptions": ["nonlethal", "default"],
    "darknessdamage_descriptions": ["never", "rare", "default", "often"],
    "lessdamagetaken_descriptions": ["always", "none", "more"],
    "riftsenabled_descriptions": ["never", "default", "always"],
}


def po_labels(payload: str) -> dict[str, dict[str, str]]:
    labels: dict[str, dict[str, str]] = {}
    payload = payload.replace("\r\n", "\n")
    pattern = re.compile(
        r'msgctxt "STRINGS\.UI\.CUSTOMIZATIONSCREEN\.([^"\\]+)"\n'
        r'msgid "([^"\\]*(?:\\.[^"\\]*)*)"\n'
        r'msgstr "([^"\\]*(?:\\.[^"\\]*)*)"'
    )
    for key, english, translated in pattern.findall(payload):
        labels[key] = {
            "en": json.loads(f'"{english}"'),
            "zh": json.loads(f'"{translated}"') if translated else json.loads(f'"{english}"'),
        }
    return labels


def parse_section(payload: str, category: str, start: str, stop: str, labels: dict[str, dict[str, str]]) -> list[dict[str, object]]:
    section = payload[payload.index(start):payload.index(stop)]
    group = ""
    group_description = ""
    options: list[dict[str, object]] = []
    for line in section.splitlines():
        match = re.match(r'\t\["([^"]+)"\] = \{', line)
        if match:
            group = match.group(1)
            group_description = ""
            continue
        match = re.match(r"\t\tdesc\s*=\s*([^,]+)", line)
        if match:
            group_description = match.group(1).strip()
            continue
        match = re.match(r'\t\t\t\["([^"]+)"\] = \{(.*)', line)
        if not match:
            continue
        key, body = match.groups()
        default_match = re.search(r'value\s*=\s*"([^"]+)"', body)
        description_match = re.search(r"desc\s*=\s*([^,}]+)", body)
        description = description_match.group(1).strip() if description_match else group_description
        world_match = re.search(r"world\s*=\s*\{([^}]+)\}", body)
        if world_match:
            worlds = re.findall(r'"([^"]+)"', world_match.group(1))
        elif re.search(r"master_controlled\s*=\s*true", body):
            worlds = ["forest"]
        else:
            worlds = ["forest", "cave"]
        if description == "tasksets.GetGenTaskLists":
            values = ["default", "classic", "cave_default"]
        elif description == "startlocations.GetGenStartLocations":
            values = ["default", "plus", "darkness", "caves"]
        else:
            values = DESCRIPTION_VALUES.get(description)
        if values is None:
            raise ValueError(f"unsupported description {description!r} for {key}")
        label = labels.get(key.upper(), {"en": key, "zh": key})
        options.append({
            "key": key,
            "label": label["zh"],
            "labelEn": label["en"],
            "category": category,
            "group": group,
            "default": default_match.group(1) if default_match else "default",
            "values": values,
            "worlds": worlds,
            "masterControlled": bool(re.search(r"master_controlled\s*=\s*true", body)),
        })
    return options


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("scripts_zip", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--build", default="unknown")
    args = parser.parse_args()
    with zipfile.ZipFile(args.scripts_zip) as archive:
        customize = archive.read("scripts/map/customize.lua").decode("utf-8")
        chinese = archive.read("scripts/languages/chinese_s.po").decode("utf-8")
    labels = po_labels(chinese)
    options = parse_section(customize, "worldgen", "local WORLDGEN_GROUP", "local WORLDGEN_MISC", labels)
    options += parse_section(customize, "worldsettings", "local WORLDSETTINGS_GROUP", "local WORLDSETTINGS_MISC", labels)
    if len(options) != 222:
        raise ValueError(f"expected 222 DST options for the pinned build, got {len(options)}")
    document = {"sourceBuild": args.build, "options": options}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(document, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
