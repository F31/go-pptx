#!/usr/bin/env python3
"""Generate and index go-pptx external corpus samples.

The tool has three subcommands:

  generate  Build ODP sources from JSON definitions and export PPTX through
            LibreOffice headless. This makes the PPTX an external-client output.
  scan      Register existing PPTX files as corpus samples without modifying
            them. Use this for private/local files that may not be committed.
  validate  Check manifest.json and <sample-id>.actions.json conventions.

Only the Python standard library is used.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import platform
import re
import shutil
import struct
import subprocess
import sys
import tempfile
import zlib
from datetime import datetime, timezone
from pathlib import Path
from typing import Any
from zipfile import ZIP_DEFLATED, ZIP_STORED, ZipFile, ZipInfo


ROOT = Path(__file__).resolve().parents[2]
SCRIPT_DIR = Path(__file__).resolve().parent
DEFAULT_OUT = ROOT / "testdata" / "corpus"


ODF_NS = {
    "office": "urn:oasis:names:tc:opendocument:xmlns:office:1.0",
    "style": "urn:oasis:names:tc:opendocument:xmlns:style:1.0",
    "text": "urn:oasis:names:tc:opendocument:xmlns:text:1.0",
    "table": "urn:oasis:names:tc:opendocument:xmlns:table:1.0",
    "draw": "urn:oasis:names:tc:opendocument:xmlns:drawing:1.0",
    "presentation": "urn:oasis:names:tc:opendocument:xmlns:presentation:1.0",
    "fo": "urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0",
    "svg": "urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0",
    "xlink": "http://www.w3.org/1999/xlink",
    "dc": "http://purl.org/dc/elements/1.1/",
    "meta": "urn:oasis:names:tc:opendocument:xmlns:meta:1.0",
    "config": "urn:oasis:names:tc:opendocument:xmlns:config:1.0",
}


def xml_escape(value: Any) -> str:
    return (
        str(value)
        .replace("&", "&amp;")
        .replace("<", "&lt;")
        .replace(">", "&gt;")
        .replace('"', "&quot;")
    )


def ns_attrs(*names: str) -> str:
    return " ".join(f'xmlns:{name}="{ODF_NS[name]}"' for name in names)


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def png_chunk(tag: bytes, payload: bytes) -> bytes:
    return struct.pack(">I", len(payload)) + tag + payload + struct.pack(">I", zlib.crc32(tag + payload) & 0xFFFFFFFF)


def solid_png(color: str = "#4472C4", width: int = 320, height: int = 180) -> bytes:
    color = color.lstrip("#")
    if len(color) != 6:
        raise ValueError(f"invalid image color: {color!r}")
    rgb = bytes(int(color[i : i + 2], 16) for i in (0, 2, 4))
    raw = b"".join(b"\x00" + rgb * width for _ in range(height))
    ihdr = struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0)
    return b"\x89PNG\r\n\x1a\n" + png_chunk(b"IHDR", ihdr) + png_chunk(b"IDAT", zlib.compress(raw, 9)) + png_chunk(b"IEND", b"")


def text_frame(name: str, x: str, y: str, w: str, h: str, paragraphs: list[str]) -> str:
    body = "".join(f"<text:p>{xml_escape(p)}</text:p>" for p in paragraphs)
    return (
        f'<draw:frame draw:name="{xml_escape(name)}" svg:x="{x}" svg:y="{y}" '
        f'svg:width="{w}" svg:height="{h}"><draw:text-box>{body}</draw:text-box></draw:frame>'
    )


def table_frame(name: str, rows: list[list[Any]]) -> str:
    cols = max((len(r) for r in rows), default=0)
    table_cols = "".join('<table:table-column table:style-name="co1"/>' for _ in range(cols))
    table_rows = []
    for row in rows:
        cells = []
        for value in row + [""] * (cols - len(row)):
            cells.append(f'<table:table-cell office:value-type="string"><text:p>{xml_escape(value)}</text:p></table:table-cell>')
        table_rows.append("<table:table-row>" + "".join(cells) + "</table:table-row>")
    return (
        f'<draw:frame draw:name="{xml_escape(name)}" table:table-name="{xml_escape(name)}" '
        'svg:x="1.4cm" svg:y="3.5cm" svg:width="22.6cm" svg:height="8.8cm">'
        f'<table:table table:name="{xml_escape(name)}">{table_cols}{"".join(table_rows)}</table:table></draw:frame>'
    )


def image_frame(name: str, href: str, x: str = "6.4cm", y: str = "4.0cm", w: str = "12.0cm", h: str = "6.8cm") -> str:
    return (
        f'<draw:frame draw:name="{xml_escape(name)}" svg:x="{x}" svg:y="{y}" svg:width="{w}" svg:height="{h}">'
        f'<draw:image xlink:href="{xml_escape(href)}" xlink:type="simple" xlink:show="embed" xlink:actuate="onLoad"/>'
        "</draw:frame>"
    )


def render_slide(slide: dict[str, Any], index: int, pictures: dict[str, bytes]) -> str:
    title = slide.get("title", f"Slide {index}")
    frames = [text_frame("Title", "1.2cm", "0.8cm", "23.0cm", "2.2cm", [title])]
    layout = slide.get("layout", "text")
    if layout in ("text", "title"):
        frames.append(text_frame("Body", "1.4cm", "3.4cm", "22.6cm", "9.0cm", slide.get("body", [])))
    elif layout == "table":
        frames.append(table_frame("Table 1", slide.get("table", [])))
    elif layout == "image":
        img = slide.get("image", {})
        path = f"Pictures/slide{index}.png"
        pictures[path] = solid_png(img.get("color", "#4472C4"), int(img.get("pixels_w", 320)), int(img.get("pixels_h", 180)))
        if slide.get("body"):
            frames.append(text_frame("Caption", "1.4cm", "11.4cm", "22.6cm", "1.6cm", slide.get("body", [])))
        frames.append(image_frame("Picture 1", path))
    else:
        raise ValueError(f"unsupported slide layout: {layout}")
    return (
        f'<draw:page draw:name="page{index}" draw:style-name="dp1" draw:master-page-name="Default">'
        + "".join(frames)
        + "</draw:page>"
    )


def build_odp(defn: dict[str, Any], out_path: Path) -> None:
    pictures: dict[str, bytes] = {}
    slides = "".join(render_slide(s, i + 1, pictures) for i, s in enumerate(defn.get("slides", [])))
    content = f'''<?xml version="1.0" encoding="UTF-8"?>
<office:document-content {ns_attrs('office','style','text','table','draw','presentation','fo','svg','xlink')} office:version="1.2">
 <office:automatic-styles>
  <style:style style:name="dp1" style:family="drawing-page"/>
 </office:automatic-styles>
 <office:body><office:presentation>{slides}</office:presentation></office:body>
</office:document-content>
'''
    styles = f'''<?xml version="1.0" encoding="UTF-8"?>
<office:document-styles {ns_attrs('office','style','text','table','draw','presentation','fo','svg')} office:version="1.2">
 <office:automatic-styles>
  <style:page-layout style:name="PL0"><style:page-layout-properties fo:page-width="25.4cm" fo:page-height="14.29cm" style:print-orientation="landscape"/></style:page-layout>
 </office:automatic-styles>
 <office:master-styles><style:master-page style:name="Default" style:page-layout-name="PL0"/></office:master-styles>
</office:document-styles>
'''
    meta = f'''<?xml version="1.0" encoding="UTF-8"?>
<office:document-meta {ns_attrs('office','meta','dc')} office:version="1.2"><office:meta><meta:generator>go-pptx scripts/gen_corpus</meta:generator><dc:title>{xml_escape(defn.get('title', defn['sample_id']))}</dc:title></office:meta></office:document-meta>
'''
    settings = f'''<?xml version="1.0" encoding="UTF-8"?>
<office:document-settings {ns_attrs('office','config')} office:version="1.2"><office:settings/></office:document-settings>
'''
    manifest_entries = [
        '<manifest:file-entry manifest:full-path="/" manifest:version="1.2" manifest:media-type="application/vnd.oasis.opendocument.presentation"/>',
        '<manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/>',
        '<manifest:file-entry manifest:full-path="styles.xml" manifest:media-type="text/xml"/>',
        '<manifest:file-entry manifest:full-path="meta.xml" manifest:media-type="text/xml"/>',
        '<manifest:file-entry manifest:full-path="settings.xml" manifest:media-type="text/xml"/>',
    ] + [f'<manifest:file-entry manifest:full-path="{p}" manifest:media-type="image/png"/>' for p in pictures]
    manifest = '<?xml version="1.0" encoding="UTF-8"?>\n<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.2">' + "".join(manifest_entries) + "</manifest:manifest>"

    out_path.parent.mkdir(parents=True, exist_ok=True)
    with ZipFile(out_path, "w") as z:
        info = ZipInfo("mimetype")
        info.compress_type = ZIP_STORED
        z.writestr(info, "application/vnd.oasis.opendocument.presentation")
        for name, data in {
            "content.xml": content,
            "styles.xml": styles,
            "meta.xml": meta,
            "settings.xml": settings,
            "META-INF/manifest.xml": manifest,
            **pictures,
        }.items():
            z.writestr(name, data, ZIP_DEFLATED)


def find_soffice(explicit: str | None) -> str | None:
    candidates = [explicit, os.environ.get("SOFFICE"), "soffice", "libreoffice"]
    for c in candidates:
        if c and shutil.which(c):
            return c
    return None


def soffice_version(cmd: str | None) -> str:
    if not cmd:
        return "LibreOffice not found"
    try:
        return subprocess.run([cmd, "--version"], check=False, text=True, capture_output=True).stdout.strip()
    except OSError as e:
        return str(e)


def convert_odp(cmd: str, odp: Path, out_dir: Path) -> Path:
    out_dir.mkdir(parents=True, exist_ok=True)
    out_arg = str(out_dir)
    odp_arg = str(odp)
    if cmd.lower().endswith(".exe"):
        # Windows LibreOffice launched from WSL does not reliably interpret
        # POSIX /mnt/<drive>/... paths. Convert them explicitly to Win32 paths.
        out_arg = subprocess.run(["wslpath", "-w", str(out_dir)], check=True, text=True, capture_output=True).stdout.strip()
        odp_arg = subprocess.run(["wslpath", "-w", str(odp)], check=True, text=True, capture_output=True).stdout.strip()
    subprocess.run([cmd, "--headless", "--convert-to", "pptx", "--outdir", out_arg, odp_arg], check=True)
    pptx = out_dir / (odp.stem + ".pptx")
    if not pptx.exists():
        raise RuntimeError(f"LibreOffice did not produce {pptx}")
    return pptx


def read_zip_text(path: Path, name: str) -> str:
    try:
        with ZipFile(path) as z:
            return z.read(name).decode("utf-8", "replace")
    except Exception:
        return ""


def zip_count(path: Path, pattern: str) -> int:
    rx = re.compile(pattern)
    try:
        with ZipFile(path) as z:
            return sum(1 for n in z.namelist() if rx.search(n))
    except Exception:
        return 0


def pptx_stats(path: Path) -> dict[str, Any]:
    app = read_zip_text(path, "docProps/app.xml")
    core = read_zip_text(path, "docProps/core.xml")
    all_ppt_xml = ""
    try:
        with ZipFile(path) as z:
            all_ppt_xml = "\n".join(
                z.read(n).decode("utf-8", "replace")
                for n in z.namelist()
                if n.startswith("ppt/") and n.endswith(".xml") and "/_rels/" not in n
            )
    except Exception:
        pass
    def tag(xml: str, name: str) -> str:
        m = re.search(rf"<{name}[^>]*>(.*?)</{name}>", xml)
        return m.group(1) if m else ""
    return {
        "application": tag(app, "Application"),
        "company": tag(app, "Company"),
        "slides": int(tag(app, "Slides") or 0),
        "creator": tag(core, "dc:creator"),
        "last_modified_by": tag(core, "cp:lastModifiedBy"),
        "media_count": zip_count(path, r"^ppt/media/"),
        "chart_count": zip_count(path, r"^ppt/charts/"),
        "table_count_hint": all_ppt_xml.count("<a:tbl"),
        "timing_count": all_ppt_xml.count("<p:timing"),
        "transition_count": all_ppt_xml.count("<p:transition"),
        "extlst_count": all_ppt_xml.count("extLst"),
    }


def feature_tags(stats: dict[str, Any]) -> list[str]:
    tags = ["real.pptx"]
    app = stats.get("application", "")
    company = stats.get("company", "")
    if "WPS" in app:
        tags.append("generator.wps")
    elif "PptxGenJS" in company:
        tags.append("generator.pptxgenjs")
    elif "PowerPoint" in app:
        tags.append("generator.powerpoint")
    if stats.get("media_count"):
        tags.append("media.image")
    if stats.get("chart_count"):
        tags.append("chart")
    if stats.get("table_count_hint"):
        tags.append("table")
    if stats.get("timing_count"):
        tags.append("animation.timing")
    if stats.get("transition_count"):
        tags.append("animation.transition")
    if stats.get("extlst_count"):
        tags.append("xml.unknown_ext")
    return tags


def manifest(sample_id: str, title: str, *, source: dict[str, Any], generator: dict[str, Any], feature_tags: list[str], expectations: list[str], known_issues: list[str], files: dict[str, Any]) -> dict[str, Any]:
    return {
        "schemaVersion": "go-pptx.corpus/1.0",
        "sample_id": sample_id,
        "title": title,
        "source_license": source,
        "generator": generator,
        "creation_steps": source.get("creation_steps", []),
        "font_environment": source.get("font_environment", "unrecorded"),
        "ooxml_type": source.get("ooxml_type", "Transitional"),
        "feature_tags": feature_tags,
        "expectations": expectations,
        "known_issues": known_issues,
        "files": files,
        "created_at": datetime.now(timezone.utc).isoformat(),
    }


def write_json(path: Path, data: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def cmd_generate(args: argparse.Namespace) -> int:
    samples_dir = Path(args.samples_dir)
    out = Path(args.out)
    soffice = find_soffice(args.soffice)
    if not soffice and not args.no_convert:
        print("LibreOffice/soffice not found. Use --no-convert to only emit ODP sources.", file=sys.stderr)
        return 2
    defs = sorted(samples_dir.glob("*.json"))
    if args.only:
        only = set(args.only)
        defs = [p for p in defs if json.loads(p.read_text(encoding="utf-8"))["sample_id"] in only]
    # Keep the conversion temp directory on the same mounted drive as the repo.
    # Windows soffice.exe launched from WSL interprets /tmp as <current-drive>:\tmp,
    # which is not the same path Python checks under WSL. A repo-local drive path
    # (for example /mnt/e/projects/...) maps consistently on both sides.
    with tempfile.TemporaryDirectory(prefix="go-pptx-corpus-", dir=str(ROOT.parent)) as tmp:
        tmp_path = Path(tmp)
        for def_path in defs:
            defn = json.loads(def_path.read_text(encoding="utf-8"))
            sid = defn["sample_id"]
            sample_dir = out / sid
            sample_dir.mkdir(parents=True, exist_ok=True)
            odp = sample_dir / f"{sid}.odp"
            build_odp(defn, odp)
            pptx_path = None
            if not args.no_convert:
                pptx_tmp = convert_odp(soffice, odp, tmp_path)
                pptx_path = sample_dir / f"{sid}.pptx"
                shutil.copy2(pptx_tmp, pptx_path)
            files = {"odp": {"path": f"{sid}.odp", "sha256": sha256_file(odp), "size": odp.stat().st_size}}
            if pptx_path:
                files["pptx"] = {"path": f"{sid}.pptx", "sha256": sha256_file(pptx_path), "size": pptx_path.stat().st_size}
            m = manifest(
                sid,
                defn.get("title", sid),
                source={
                    "source": "scripts/gen_corpus generated ODP exported by LibreOffice",
                    "license": defn.get("license", "project-owned/generated; verify before redistribution"),
                    "redistributable": defn.get("redistributable", True),
                    "creation_steps": ["python3 scripts/gen_corpus/corpus.py generate", "LibreOffice --headless --convert-to pptx"],
                },
                generator={"name": "LibreOffice", "version": soffice_version(soffice), "platform": platform.platform()},
                feature_tags=defn.get("feature_tags", []),
                expectations=defn.get("expectations", ["OpenReader succeeds", "validate reports no structural errors"]),
                known_issues=defn.get("known_issues", []),
                files=files,
            )
            write_json(sample_dir / "manifest.json", m)
            print(f"generated {sid}: {sample_dir}")
    return 0


def cmd_scan(args: argparse.Namespace) -> int:
    src = Path(args.source)
    out = Path(args.out)
    files = sorted(src.rglob("*.pptx") if args.recursive else src.glob("*.pptx"))
    if not files:
        print(f"no pptx files found in {src}", file=sys.stderr)
        return 1
    for i, pptx in enumerate(files, 1):
        sid = f"ext-{i:04d}"
        sample_dir = out / sid
        sample_dir.mkdir(parents=True, exist_ok=True)
        dst_name = f"{sid}.pptx"
        if args.copy:
            shutil.copy2(pptx, sample_dir / dst_name)
            stored_path = dst_name
        else:
            stored_path = str(pptx)
        stats = pptx_stats(pptx)
        m = manifest(
            sid,
            pptx.stem,
            source={
                "source": str(pptx),
                "license": args.license,
                "redistributable": args.redistributable,
                "creation_steps": ["registered by scripts/gen_corpus/corpus.py scan"],
                "font_environment": args.font_environment,
            },
            generator={"name": stats.get("application") or "unknown", "version": "unrecorded", "platform": "unrecorded", "raw": stats},
            feature_tags=feature_tags(stats),
            expectations=["OpenReader succeeds", "validate reports no structural errors", "no-edit Write keeps unchanged parts byte-identical"],
            known_issues=[] if args.license != "unrecorded/private" else ["license and redistribution permission must be confirmed before committing the PPTX file"],
            files={"pptx": {"path": stored_path, "sha256": sha256_file(pptx), "size": pptx.stat().st_size}},
        )
        write_json(sample_dir / "manifest.json", m)
        print(f"indexed {sid}: {pptx.name} tags={','.join(m['feature_tags'])}")
    return 0


REQUIRED_MANIFEST_FIELDS = ["sample_id", "source_license", "generator", "feature_tags", "expectations", "files"]
ACTION_REQUIREMENTS = {
    "ReplaceText": ["old", "new"],
    "SetPlainText": ["text"],
    "SetNotes": ["text"],
    "Bind": ["data"],
}


def validate_sample(sample_dir: Path) -> list[str]:
    errors: list[str] = []
    manifest_path = sample_dir / "manifest.json"
    if not manifest_path.exists():
        return [f"{sample_dir}: missing manifest.json"]
    try:
        m = json.loads(manifest_path.read_text(encoding="utf-8"))
    except Exception as e:
        return [f"{manifest_path}: invalid json: {e}"]
    sid = m.get("sample_id")
    for field in REQUIRED_MANIFEST_FIELDS:
        if field not in m:
            errors.append(f"{manifest_path}: missing {field}")
    if sid and sid != sample_dir.name:
        errors.append(f"{manifest_path}: sample_id {sid!r} does not match directory {sample_dir.name!r}")
    actions_path = sample_dir / f"{sid}.actions.json"
    edited_path = sample_dir / f"{sid}.edited.pptx"
    if actions_path.exists() != edited_path.exists():
        errors.append(f"{sample_dir}: actions.json and edited.pptx must appear as a pair")
    if actions_path.exists():
        try:
            actions = json.loads(actions_path.read_text(encoding="utf-8"))
        except Exception as e:
            errors.append(f"{actions_path}: invalid json: {e}")
            actions = []
        if isinstance(actions, dict):
            actions = [actions]
        if not isinstance(actions, list) or not actions:
            errors.append(f"{actions_path}: must be a non-empty object or array")
        for idx, action in enumerate(actions):
            if not isinstance(action, dict):
                errors.append(f"{actions_path}: action[{idx}] must be object")
                continue
            kind = action.get("action")
            if kind not in ACTION_REQUIREMENTS:
                errors.append(f"{actions_path}: action[{idx}] unsupported action {kind!r}")
                continue
            for req in ACTION_REQUIREMENTS[kind]:
                if req not in action:
                    errors.append(f"{actions_path}: action[{idx}] {kind} missing {req}")
    return errors


def cmd_validate(args: argparse.Namespace) -> int:
    root = Path(args.root)
    samples = [root] if (root / "manifest.json").exists() else sorted(p for p in root.iterdir() if p.is_dir())
    errors: list[str] = []
    for sample in samples:
        errors.extend(validate_sample(sample))
    for err in errors:
        print(err, file=sys.stderr)
    print(f"validated {len(samples)} sample(s), errors={len(errors)}")
    return 1 if errors else 0


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="cmd", required=True)

    gen = sub.add_parser("generate", help="build ODP sources and export PPTX through LibreOffice")
    gen.add_argument("--samples-dir", default=str(SCRIPT_DIR / "samples"))
    gen.add_argument("--out", default=str(DEFAULT_OUT))
    gen.add_argument("--soffice")
    gen.add_argument("--no-convert", action="store_true")
    gen.add_argument("--only", nargs="*")
    gen.set_defaults(func=cmd_generate)

    scan = sub.add_parser("scan", help="index existing local PPTX files as external corpus samples")
    scan.add_argument("source")
    scan.add_argument("--out", default=str(DEFAULT_OUT))
    scan.add_argument("--recursive", action="store_true")
    scan.add_argument("--copy", action="store_true", help="copy PPTX files into corpus dirs; otherwise manifests reference the source path")
    scan.add_argument("--license", default="unrecorded/private")
    scan.add_argument("--redistributable", action="store_true")
    scan.add_argument("--font-environment", default="unrecorded")
    scan.set_defaults(func=cmd_scan)

    val = sub.add_parser("validate", help="validate manifest/actions corpus convention")
    val.add_argument("root", nargs="?", default=str(DEFAULT_OUT))
    val.set_defaults(func=cmd_validate)

    args = parser.parse_args(argv)
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
