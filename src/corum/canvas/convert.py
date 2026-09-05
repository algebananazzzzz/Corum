"""Convert Canvas HTML into raw Markdown capture files."""

from __future__ import annotations

import re
from datetime import UTC, datetime, tzinfo
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit
from zoneinfo import ZoneInfo

import yaml
from bs4 import BeautifulSoup
from markdownify import markdownify


_NOISE_CLASSES = ("external_link_icon", "screenreader-only")
_MOBILE_CONFIG = "BlueCanvasMobileConfig.js"


def local_time(raw: str | None, timezone: str | tzinfo = UTC) -> str | None:
    """Return a Canvas instant in the selected workspace timezone."""
    if not raw:
        return None
    zone = ZoneInfo(timezone) if isinstance(timezone, str) else timezone
    return datetime.fromisoformat(raw.replace("Z", "+00:00")).astimezone(zone).isoformat()


def strip_verifier(url: str) -> str:
    """Remove Canvas's per-user file verifier from a URL."""
    parts = urlsplit(url)
    if not parts.query:
        return url
    kept = [(key, value) for key, value in parse_qsl(parts.query, keep_blank_values=True) if key != "verifier"]
    return urlunsplit(parts._replace(query=urlencode(kept)))


def slug(text: str, limit: int = 60) -> str:
    """Return a filename-safe slug while retaining lesson-number dots."""
    cleaned = re.sub(r"[^\w.\s-]", "", (text or "").strip().lower())
    cleaned = re.sub(r"[\s_-]+", "-", cleaned).strip("-.")
    if len(cleaned) <= limit:
        return cleaned or "untitled"
    cut = cleaned[:limit]
    if cleaned[limit] != "-":
        cut = cut.rsplit("-", 1)[0]
    return cut.strip("-.") or cleaned[:limit]


def _scrub(soup: BeautifulSoup) -> None:
    for script in soup.find_all("script"):
        if _MOBILE_CONFIG in (script.get("src") or ""):
            script.decompose()
    for tag in soup.find_all(True):
        classes = tag.get("class") or []
        if any(item in _NOISE_CLASSES for item in classes):
            tag.decompose()
            continue
        for attribute in [name for name in tag.attrs if name.startswith("data-")]:
            del tag[attribute]
        tag.attrs.pop("class", None)
        for attribute in ("href", "src"):
            if tag.get(attribute):
                tag[attribute] = strip_verifier(tag[attribute])
        if tag.name == "span" and not tag.attrs:
            tag.unwrap()


def links(html: str, canvas_host: str | None = None) -> list[dict[str, str]]:
    """Return unique links in a Canvas HTML body without verifiers."""
    soup = BeautifulSoup(html or "", "html.parser")
    _scrub(soup)
    output: list[dict[str, str]] = []
    seen: set[str] = set()
    for anchor in soup.find_all("a", href=True):
        href = strip_verifier(anchor["href"])
        if href in seen or href.startswith("#"):
            continue
        seen.add(href)
        kind = "file" if "/files/" in href else "internal" if canvas_host and canvas_host in href else "external"
        output.append({"type": kind, "text": " ".join(anchor.get_text().split()) or href, "url": href})
    return output


def images(html: str) -> list[str]:
    """Return inline image URLs from a Canvas HTML body without verifiers."""
    soup = BeautifulSoup(html or "", "html.parser")
    return [strip_verifier(image["src"]) for image in soup.find_all("img", src=True)]


def to_markdown(html: str, image_paths: dict[str, str] | None = None) -> str:
    """Render a scrubbed Canvas body as stable Markdown."""
    soup = BeautifulSoup(html or "", "html.parser")
    _scrub(soup)
    for image in soup.find_all("img", src=True):
        local = (image_paths or {}).get(strip_verifier(image["src"]))
        if local:
            image["src"] = local
    text = markdownify(str(soup), heading_style="ATX", bullets="-")
    text = re.sub(r"[ \t]+\n", "\n", text)
    text = re.sub(r"\n{3,}", "\n\n", text)
    return text.strip() + "\n"


def frontmatter(fields: dict, link_list: list[dict] | None = None) -> str:
    """Render raw-material frontmatter from scalar fields and links."""
    class _FrontmatterDumper(yaml.SafeDumper):
        def increase_indent(self, flow=False, indentless=False):
            return super().increase_indent(flow, False)

    def represent_string(dumper, value):
        unsafe_plain = "\n" in value or ": " in value or value.strip().casefold() in {
            "yes",
            "no",
            "true",
            "false",
            "null",
            "on",
            "off",
        }
        return dumper.represent_scalar(
            "tag:yaml.org,2002:str", value, style='"' if unsafe_plain else None
        )

    _FrontmatterDumper.add_representer(str, represent_string)
    payload = {key: value for key, value in fields.items() if value is not None}
    if link_list:
        payload["links"] = link_list
    document = yaml.dump(
        payload,
        Dumper=_FrontmatterDumper,
        allow_unicode=True,
        default_flow_style=False,
        sort_keys=False,
    ).rstrip()
    return f"---\n{document}\n---\n\n"
