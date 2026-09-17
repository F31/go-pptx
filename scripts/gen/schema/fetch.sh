#!/usr/bin/env bash
# fetch.sh 下载并解包 ECMA-376 第 5 版 Transitional XML Schema 到目标目录。
#
# 用法：scripts/gen/schema/fetch.sh [outdir]
# 默认 outdir：internal/ooxml/schema/.xsd（.gitignore 忽略，不入库）。
#
# 来源：ECMA-376 第 5 版 Part 4（Transitional Migration Features）所附
# OfficeOpenXML-XMLSchema-Transitional.zip。命名空间为
# schemas.openxmlformats.org（与真实 PPTX 一致）；Part 1 附的是 Strict
# （purl.oclc.org），不可用于本库。
set -euo pipefail

outdir="${1:-internal/ooxml/schema/.xsd}"
url="https://www.ecma-international.org/wp-content/uploads/ECMA-376-4_5th_edition_december_2016.zip"

mkdir -p "$outdir"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "downloading $url"
curl -fsSL -o "$tmp/part4.zip" "$url"
unzip -o -q "$tmp/part4.zip" OfficeOpenXML-XMLSchema-Transitional.zip -d "$tmp"
unzip -o -q "$tmp/OfficeOpenXML-XMLSchema-Transitional.zip" -d "$outdir"

echo "extracted $(ls "$outdir"/*.xsd | wc -l) XSD files to $outdir"
