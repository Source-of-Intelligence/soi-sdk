module soi.dev/xlsx2md

go 1.22.0

require (
	soi.dev/soi-sdk v1.0.0
	soi.dev/soi-vos v0.0.0
)

replace (
	soi.dev/soi-sdk => ../../
	soi.dev/soi-vos => ../../../soi-vos
)
