from __future__ import annotations

from datetime import datetime
from email.utils import parsedate_to_datetime
from xml.etree import ElementTree


def normalize_source_url(value: str | None) -> str | None:
    if not value:
        return None
    value = value.strip()
    return value or None


def parse_datetime(value: str | None) -> datetime | None:
    if not value:
        return None
    try:
        return parsedate_to_datetime(value)
    except (TypeError, ValueError):
        try:
            return datetime.fromisoformat(value.replace("Z", "+00:00"))
        except ValueError:
            return None


def strip_html(value: str | None) -> str | None:
    if not value:
        return None
    return " ".join(value.replace("<p>", " ").replace("</p>", " ").replace("<br>", " ").split())


def xml_text(item: ElementTree.Element, tag: str) -> str | None:
    element = item.find(tag)
    if element is None or element.text is None:
        return None
    return element.text.strip()


def rss_link(item: ElementTree.Element) -> str | None:
    link = xml_text(item, "link")
    if link:
        return link
    atom_link = item.find("{http://www.w3.org/2005/Atom}link")
    if atom_link is not None:
        return atom_link.attrib.get("href")
    return None

