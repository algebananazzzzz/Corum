import yaml
from bs4 import BeautifulSoup

from corum.canvas import convert


def test_slug_keeps_dots_so_lesson_numbers_stay_readable():
    assert convert.slug("Lesson 1.3") == "lesson-1.3"
    assert convert.slug("Lesson 5: Elderly befriending") == "lesson-5-elderly-befriending"


def test_slug_truncates_at_a_word_boundary():
    assert convert.slug("a" * 30 + " " + "b" * 40, limit=40) == "a" * 30


def test_slug_keeps_the_full_word_when_the_cut_lands_on_a_separator():
    assert convert.slug("aaa bbb cc", limit=7) == "aaa-bbb"


def test_strip_verifier_removes_only_the_session_secret():
    url = "https://c.edu/files/1/download?download_frd=1&verifier=abc-123"
    assert convert.strip_verifier(url) == "https://c.edu/files/1/download?download_frd=1"


def test_strip_verifier_leaves_a_query_less_url_alone():
    assert convert.strip_verifier("https://c.edu/files/1") == "https://c.edu/files/1"


def test_scrub_unwraps_a_bare_span_but_drops_a_screenreader_only_one():
    soup = BeautifulSoup(
        '<p>Hi<span> there</span></p><span class="screenreader-only">skip</span>',
        "html.parser",
    )
    convert._scrub(soup)
    assert soup.find("span") is None
    assert soup.get_text() == "Hi there"


def test_scrub_drops_the_mobile_config_script_canvas_appends_to_every_body():
    soup = BeautifulSoup(
        '<p>x</p><script src="https://c.edu/dist/BlueCanvasMobileConfig.js">var x=1;</script>',
        "html.parser",
    )
    convert._scrub(soup)
    assert soup.find("script") is None


def test_to_markdown_leaves_no_trailing_double_space_line_breaks():
    assert "  \n" not in convert.to_markdown("<p>a<br>b</p>")


def test_to_markdown_rewrites_a_downloaded_image_to_its_local_path():
    html = '<p><img src="https://c.edu/files/9/preview?verifier=zz"></p>'
    out = convert.to_markdown(html, {"https://c.edu/files/9/preview": "image.png"})
    assert "image.png" in out and "verifier" not in out


def test_links_types_targets_and_drops_duplicates():
    html = (
        '<a href="https://c.edu/files/3/download?verifier=q">Slides</a>'
        '<a href="https://forms.gle/x">Form</a>'
        '<a href="https://forms.gle/x">Form again</a>'
    )
    assert convert.links(html) == [
        {"type": "file", "text": "Slides", "url": "https://c.edu/files/3/download"},
        {"type": "external", "text": "Form", "url": "https://forms.gle/x"},
    ]


def test_images_finds_bodies_canvas_leaves_out_of_the_attachments_array():
    assert convert.images('<img src="https://c.edu/f/1?verifier=k">') == ["https://c.edu/f/1"]


def test_frontmatter_omits_empty_fields_and_renders_links():
    out = convert.frontmatter(
        {"source": "canvas", "kind": "announcement", "id": 1, "author": None},
        [{"type": "external", "text": "F", "url": "https://x"}],
    )
    assert "author" not in out
    assert out.startswith("---\nsource: canvas\n")
    assert "  - type: external\n    text: F\n    url: https://x\n" in out


def test_frontmatter_safe_serializes_canvas_controlled_scalars():
    out = convert.frontmatter(
        {
            "source": "canvas",
            "title": "Deadline: Friday\n---\nnot: frontmatter",
            "enabled_like": "yes",
        }
    )

    document, body = out.removeprefix("---\n").rsplit("\n---\n", 1)
    assert yaml.safe_load(document) == {
        "source": "canvas",
        "title": "Deadline: Friday\n---\nnot: frontmatter",
        "enabled_like": "yes",
    }
    assert body == "\n"


def test_canvas_timestamp_conversion_honors_selected_timezone():
    assert convert.local_time(
        "2026-09-06T01:30:00Z", "America/New_York"
    ) == "2026-09-05T21:30:00-04:00"
