from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[1]
OWNERS = {
    "EMMA": "emma",
    "Fronius": "fronius",
    "Growatt aggregate": "growatt-aggregate",
    "LUNA": "luna",
    "OutBack": "outback",
    "S-Dongle": "sdongle",
    "SmartLogger": "smartlogger",
    "SUN2000": "sun2000",
}
PUBLIC_OWNER = "https://github.com/Project-Helianthus/helianthus-docs-modbus/blob/main/architecture/"


def owner_url(family):
    return PUBLIC_OWNER + family + "-provider-port.md"


def validate_owner_links(text, labels):
    links = re.findall(r"\[([^\]]+)\]\(([^)]+)\)", text)
    for label in labels:
        if (label, owner_url(OWNERS[label])) not in links:
            raise ValueError("missing canonical provider-port owner: " + label)
    for _, target in links:
        if any(family + "-provider-port.md" in target for family in OWNERS.values()):
            if target not in {owner_url(family) for family in OWNERS.values()}:
                raise ValueError("noncanonical provider-port link")


class ProviderPortLinksTests(unittest.TestCase):
    def test_duplicate_pages_are_absent(self):
        for family in OWNERS.values():
            with self.subTest(family=family):
                self.assertFalse((ROOT / "docs" / (family + "-provider-port.md")).exists())

    def test_readme_links_every_owner(self):
        validate_owner_links((ROOT / "README.md").read_text(), OWNERS)

    def test_huawei_overview_links_gateway_owners(self):
        validate_owner_links(
            (ROOT / "docs/huawei-readonly-providers.md").read_text(),
            ("SmartLogger", "EMMA", "S-Dongle"),
        )

    def test_remaining_markdown_has_no_duplicate_page_links(self):
        for path in [ROOT / "README.md", *(ROOT / "docs").rglob("*.md")]:
            with self.subTest(path=path.relative_to(ROOT)):
                validate_owner_links(path.read_text(), ())

    def test_each_owner_and_url_field_is_checked(self):
        for label, family in OWNERS.items():
            url = owner_url(family)
            valid = f"[{label}]({url})"
            validate_owner_links(valid, (label,))
            mutations = {
                "label": valid.replace(f"[{label}]", "[Other]"),
                "scheme": valid.replace("https:", "http:"),
                "host": valid.replace("github.com", "example.invalid"),
                "organization": valid.replace("Project-Helianthus", "Other"),
                "repository": valid.replace("helianthus-docs-modbus", "helianthus-modbusreg"),
                "route": valid.replace("/blob/", "/tree/"),
                "branch": valid.replace("/main/", "/other/"),
                "directory": valid.replace("/architecture/", "/docs/"),
                "family": valid.replace(family + "-provider-port.md", "other-provider-port.md"),
                "query": valid.replace(".md)", ".md?raw=1)"),
                "fragment": valid.replace(".md)", ".md#section)"),
            }
            for field, mutated in mutations.items():
                with self.subTest(family=family, field=field):
                    with self.assertRaises(ValueError):
                        validate_owner_links(mutated, (label,))

    def test_missing_owner_and_extra_local_link_are_rejected(self):
        with self.assertRaises(ValueError):
            validate_owner_links("", ("EMMA",))
        with self.assertRaises(ValueError):
            validate_owner_links(
                f"[EMMA]({owner_url('emma')}) [copy](docs/emma-provider-port.md)",
                ("EMMA",),
            )
