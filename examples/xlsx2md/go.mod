module github.com/Source-of-Intelligence/soi/xlsx2md

go 1.22.0

require (
	github.com/Source-of-Intelligence/soi-sdk v1.0.0
	github.com/Source-of-Intelligence/soi-vos v0.0.0
)

replace (
	github.com/Source-of-Intelligence/soi-sdk => ../../
	github.com/Source-of-Intelligence/soi-vos => ../../../soi-vos
)
