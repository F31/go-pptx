#!/bin/bash
# L3 媒体兼容矩阵：{audio-auto, audio-click, video} × {PowerPoint, WPS}。
#
# 目的：验证 ADR-027 的"媒体四态"——每个 fixture 需人工确认：
#   ① 客户端能打开（无修复提示）  ② 喇叭/视频图标可见
#   ③ 图标可点击  ④ 播放出声（auto 自动 / click 单击）
#
# 用法（WSL，Windows 宿主机；需装了 PowerPoint 与/或 WPS）：
#   bash scripts/l3/media_matrix.sh
#
# 自动部分：generator 出 fixture → COM 打开/重存（run_client.sh）。
# 人工部分：脚本末尾打印逐项人工核对清单（播放效果无法自动化）。
set -u

ROOT_WIN='E:\projects\go-pptx'
OUT_DIR='.l3-output/media'
WIN_OUT="$ROOT_WIN\\.l3-output\\media"

echo "== 1) 生成 fixtures =="
go run ./scripts/gen_media -out "$OUT_DIR" || { echo "fixture generation failed"; exit 1; }

echo
echo "== 2) COM 打开/重存矩阵 =="
printf '%-14s %-4s %-50s\n' "sample" "app" "result"
for sample in audio-auto audio-click video; do
  for app in ppt wpp; do
    src="$WIN_OUT\\${sample}.pptx"
    dst="$WIN_OUT\\${sample}.${app}-resaved.pptx"
    result=$(bash scripts/l3/run_client.sh "$app" "$src" "$dst" 2>&1 | tr '\n' ' ')
    printf '%-14s %-4s %-50s\n' "$sample" "$app" "$result"
  done
done

echo
echo "== 3) 人工核对清单（COM 无法验证播放） =="
cat <<'EOF'
  audio-auto.pptx   : 图标可见(F5 前) + F5 自动出声（无需点击）
  audio-click.pptx  : 图标可见 + F5 后单击图标出声
  video.pptx        : 图标可见 + F5 播放视频画面

  两家客户端（PowerPoint / WPS）都要过。
  PowerPoint 偏严格（判损）而 WPS 偏宽容；两者故障模式不同，缺一不可。
EOF
