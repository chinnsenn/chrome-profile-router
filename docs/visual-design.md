# 视觉来源

选中身份的流彩描边与柔光参考了三个开源项目的参数与结构，Go 实现是独立重写。链接固定到具体 commit，避免上游改动后对不上。

| 来源（固定 commit） | 参考的原值 | 本项目 |
| --- | --- | --- |
| Stripe [`ba6f5ed`](https://github.com/stripe/stripe-ios/blob/ba6f5ed8b3011c3d8223fd9b6b0ee35d2e55e6af/StripeIdentity/StripeIdentity/Source/NativeComponents/Views/DocumentCapture/AnimatedBorderView.swift) `AnimatedBorderView` | stop 位置 `[0, 0.12, 0.55, 0.75, 1]`；亮弧 alpha 结构 `[c1, c2@0, c2@0, c1, c1]` | 透明 stop 与基础选中边框预合成，任何相位都不丢边框 |
| Magic UI [`efe76fc`](https://github.com/magicuidesign/magicui/blob/efe76fc5755eca1142dd4f74c1d6f0ace2887e3c/apps/www/registry/magicui/border-beam.tsx) `BorderBeam` | 周期 6s 线性无限；线宽 1；暖色 `#FFAA40`、紫 `#9C40FF` | 只改渐变起始角，不旋转矩形；首尾 stop 取同一暖色，转圈接缝连续 |
| Pow [`1b4b1dd`](https://github.com/EmergeTools/Pow/blob/1b4b1dda28c50b95f0872927ee2226fe8b58950e/Sources/Pow/Effects/GlowEffect.swift) `GlowEffect` | `amount=0.7`；alpha `sqrt(amount)/{1.2,4}`；半径 `amount*R/{4,2}`；偏移 `(0,0)` | 只取内两级，`R=19`、alpha 再乘 `0.62`，半径 3.325 / 6.650pt；`R` 与 `gain` 是按真机像素调出的适配值，不是来源原值（Pow 对应默认为 16 与 1） |

三处结构差异：Stripe 与 BorderBeam 都用图层加 mask 只留 1px 环，这里直接用 `BorderConicGradient` 画环；Stripe 靠旋转整张渐变层推进，这里只改渐变起始角，矩形不旋转；主题 accent 不由渐变 stop 承担，而由基础选中边框与两级柔光体现。

无障碍：系统开启减少动态效果时角度停在 0°，静态描边与柔光照常保留；高对比、降低透明度或窗口无材质时退回实色表面加 1pt 描边，不出渐变与柔光。以上项目各自的开源许可见其仓库，这里只记录参数与结构的对应关系。常量都在 [`card_glow.go`](../card_glow.go)。
