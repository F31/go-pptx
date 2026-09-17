package pptx

// A-2 编译期断言：所有具体形状类型均满足对应的能力窄接口（经由 shapeNode 或
// 自身方法实现）。任一类型不再满足时，本文件编译失败，及时暴露接口契约漂移。
var (
	_ GeometryProvider        = (*AutoShape)(nil)
	_ FillProvider            = (*AutoShape)(nil)
	_ EffectsProvider         = (*AutoShape)(nil)
	_ StyleMatrixRefsProvider = (*AutoShape)(nil)
	_ LineProvider            = (*AutoShape)(nil)

	_ GeometryProvider        = (*TextShape)(nil)
	_ FillProvider            = (*TextShape)(nil)
	_ EffectsProvider         = (*TextShape)(nil)
	_ StyleMatrixRefsProvider = (*TextShape)(nil)
	_ LineProvider            = (*TextShape)(nil)

	_ GeometryProvider        = (*PictureShape)(nil)
	_ FillProvider            = (*PictureShape)(nil)
	_ EffectsProvider         = (*PictureShape)(nil)
	_ StyleMatrixRefsProvider = (*PictureShape)(nil)
	_ LineProvider            = (*PictureShape)(nil)

	_ GeometryProvider        = (*ChartShape)(nil)
	_ FillProvider            = (*ChartShape)(nil)
	_ EffectsProvider         = (*ChartShape)(nil)
	_ StyleMatrixRefsProvider = (*ChartShape)(nil)
	_ LineProvider            = (*ChartShape)(nil)

	_ GeometryProvider        = (*GroupShape)(nil)
	_ FillProvider            = (*GroupShape)(nil)
	_ EffectsProvider         = (*GroupShape)(nil)
	_ StyleMatrixRefsProvider = (*GroupShape)(nil)
	_ LineProvider            = (*GroupShape)(nil)

	_ GeometryProvider        = (*TableShape)(nil)
	_ FillProvider            = (*TableShape)(nil)
	_ EffectsProvider         = (*TableShape)(nil)
	_ StyleMatrixRefsProvider = (*TableShape)(nil)
	_ LineProvider            = (*TableShape)(nil)

	_ GeometryProvider        = (*AudioShape)(nil)
	_ FillProvider            = (*AudioShape)(nil)
	_ EffectsProvider         = (*AudioShape)(nil)
	_ StyleMatrixRefsProvider = (*AudioShape)(nil)
	_ LineProvider            = (*AudioShape)(nil)

	_ GeometryProvider        = (*VideoShape)(nil)
	_ FillProvider            = (*VideoShape)(nil)
	_ EffectsProvider         = (*VideoShape)(nil)
	_ StyleMatrixRefsProvider = (*VideoShape)(nil)
	_ LineProvider            = (*VideoShape)(nil)

	_ GeometryProvider        = (*OpaqueShape)(nil)
	_ FillProvider            = (*OpaqueShape)(nil)
	_ EffectsProvider         = (*OpaqueShape)(nil)
	_ StyleMatrixRefsProvider = (*OpaqueShape)(nil)
	_ LineProvider            = (*OpaqueShape)(nil)
)
