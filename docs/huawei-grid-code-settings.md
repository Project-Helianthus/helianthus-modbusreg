# Huawei grid-code settings

These rows have no SemReg element. A host calls them by hand in the modbusreg namespace. This package does not send a bus command. A read returns the card's holding-register span, and a write returns register words. A rejected call returns no words.

The type, gain, unit, address and quantity are the catalogue card's. The gain token stays on the row and is not applied here. A token that is not one positive integer stays as the card wrote it. A row the card does not name is addressed by register and type. A block whose interior the card does not give is that register span as raw words. A row whose quantity disagrees with its type is listed and admits neither a read nor a write.

A read-only row has a read and no write. A write-only row has a write and no read. A read-write row reads back through the same row.

The companion page is `protocols/huawei/grid-code-settings-v1.md` in helianthus-docs-modbus. A row in this table is defined by its catalogue card. No hardware-test result is recorded here.

## L-LEGACY

L-LEGACY defines no grid-code row

## L-V200

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| l-v200.control.grid_protection.42000 |  | 42000 | 1 | U16 | not stated | not stated | RW | yes |
| l-v200.control.grid_protection.42015 |  | 42015 | 2 | U32 | 1000 | %/s | RW | yes |
| l-v200.control.grid_protection.42017 |  | 42017 | 2 | U32 | 1000 | %/s | RW | yes |
| l-v200.control.grid_protection.42019 |  | 42019 | 2 | U32 | 1 | s | RW | yes |
| l-v200.control.grid_protection.42405 |  | 42405 | 2 | I32 | 1000 | kW | RW | yes |

## LB-C00

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| lb-c00.control.grid_protection.42000 |  | 42000 | 1 | U16 | not stated | not stated | RW | yes |
| lb-c00.control.grid_protection.42015 |  | 42015 | 2 | U32 | 1000 | %/s | RW | yes |
| lb-c00.control.grid_protection.42017 |  | 42017 | 2 | U32 | 1000 | %/s | RW | yes |
| lb-c00.control.grid_protection.42019 |  | 42019 | 2 | U32 | 1 | s | RW | yes |
| lb-c00.control.grid_protection.42405 |  | 42405 | 2 | I32 | 1000 | kW | RW | yes |

## LB-C10

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| lb-c10.control.grid_protection.42000 |  | 42000 | 1 | U16 | not stated | not stated | RW | yes |
| lb-c10.control.grid_protection.42015 |  | 42015 | 2 | U32 | 1000 | %/s | RW | yes |
| lb-c10.control.grid_protection.42017 |  | 42017 | 2 | U32 | 1000 | %/s | RW | yes |
| lb-c10.control.grid_protection.42019 |  | 42019 | 2 | U32 | 1 | s | RW | yes |
| lb-c10.control.grid_protection.42405 |  | 42405 | 2 | I32 | 1000 | kW | RW | yes |

## LC

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| lc.control.grid_protection.42000 |  | 42000 | 1 | U16 | not stated | not stated | RW | yes |
| lc.control.grid_protection.42015 |  | 42015 | 2 | U32 | 1000 | %/s | RW | yes |
| lc.control.grid_protection.42017 |  | 42017 | 2 | U32 | 1000 | %/s | RW | yes |
| lc.control.grid_protection.42019 |  | 42019 | 2 | U32 | 1 | s | RW | yes |
| lc.control.grid_protection.42405 |  | 42405 | 2 | I32 | 1000 | kW | RW | yes |

## MA-C00

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| ma-c00.control.grid_protection.42000 |  | 42000 | 1 | U16 | not stated | not stated | RW | yes |
| ma-c00.control.grid_protection.42015 |  | 42015 | 2 | U32 | 1000 | %/s | RW | yes |
| ma-c00.control.grid_protection.42017 |  | 42017 | 2 | U32 | 1000 | %/s | RW | yes |
| ma-c00.control.grid_protection.42019 |  | 42019 | 2 | U32 | 1 | s | RW | yes |
| ma-c00.control.grid_protection.42405 |  | 42405 | 2 | I32 | 1000 | kW | RW | yes |

## MA-C20

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| identity.version_component.powergridstandardcodeprotocolversion |  | 30109 | 1 | U16 | not stated | not stated | RO | no |
| ma-c20.control.grid_protection.42000 |  | 42000 | 1 | U16 | 1 | not stated | RW | yes |
| ma-c20.control.grid_protection.42001 |  | 42001 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42002 |  | 42002 | 1 | U16 | 1 | V | RW | yes |
| ma-c20.control.grid_protection.42003 |  | 42003 | 1 | U16 | 1 | Hz | RW | yes |
| ma-c20.control.grid_protection.42012 |  | 42012 | 1 | U16 | 1 | not stated | RW | yes |
| ma-c20.control.grid_protection.42014 |  | 42014 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42015 |  | 42015 | 2 | U32 | 1000 | %/s | RW | yes |
| ma-c20.control.grid_protection.42017 |  | 42017 | 2 | U32 | 1000 | %/s | RW | yes |
| ma-c20.control.grid_protection.42019 |  | 42019 | 2 | U32 | 1 | s | RW | yes |
| ma-c20.control.grid_protection.42021 |  | 42021 | 2 | U32 | 1000 | kVA | RW | yes |
| ma-c20.control.grid_protection.42023 |  | 42023 | 2 | U32 | 1000 | kW | RW | yes |
| ma-c20.control.grid_protection.42025 |  | 42025 | 2 | U32 | 1000 | kVar | RW | yes |
| ma-c20.control.grid_protection.42027 |  | 42027 | 2 | U32 | 1000 | kW | RW | yes |
| ma-c20.control.grid_protection.42029 |  | 42029 | 1 | U16 | 1 | min/100% | RW | yes |
| ma-c20.control.grid_protection.42030 |  | 42030 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42032 |  | 42032 | 1 | U16 | 10 | s | RW | yes |
| ma-c20.control.grid_protection.42033 |  | 42033 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42034 |  | 42034 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42040 |  | 42040 | 1 | U16 | 1 | s | RW | yes |
| ma-c20.control.grid_protection.42041 |  | 42041 | 1 | U16 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42042 |  | 42042 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42043 |  | 42043 | 1 | U16 | 1000 | Hz | RW | yes |
| ma-c20.control.grid_protection.42044 |  | 42044 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42045 |  | 42045 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42046 |  | 42046 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42047 |  | 42047 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42048 |  | 42048 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42049 |  | 42049 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42050 |  | 42050 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42051 |  | 42051 | 1 | U16 | 1 | %/min | RW | yes |
| ma-c20.control.grid_protection.42052 |  | 42052 | 1 | U16 | 10 | % | RW | yes |
| ma-c20.control.grid_protection.42053 |  | 42053 | 1 | U16 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42054 |  | 42054 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42055 |  | 42055 | 1 | U16 | 1 | min | RW | yes |
| ma-c20.control.grid_protection.42061 |  | 42061 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42062 |  | 42062 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42063 |  | 42063 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42064 |  | 42064 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42065 |  | 42065 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42066 |  | 42066 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42067 |  | 42067 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42068 |  | 42068 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42069 |  | 42069 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42070 |  | 42070 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42072 |  | 42072 | 1 | U16 | 1 | s | RW | yes |
| ma-c20.control.grid_protection.42073 |  | 42073 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42074 |  | 42074 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42075 |  | 42075 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42076 |  | 42076 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42077 |  | 42077 | 2 | U32 | 10 | kW | RW | yes |
| ma-c20.control.grid_protection.42079 |  | 42079 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42080 |  | 42080 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42081 |  | 42081 | 1 | U16 | 10 | Hz/s | RW | yes |
| ma-c20.control.grid_protection.42082 |  | 42082 | 1 | U16 | 10 | s | RW | yes |
| ma-c20.control.grid_protection.42083 |  | 42083 | 1 | I16 | 1000 | not stated | RW | yes |
| ma-c20.control.grid_protection.42084 |  | 42084 | 1 | U16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42085 |  | 42085 | 1 | U16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42086 |  | 42086 | 1 | U16 | 100 | Hz | RW | yes |
| ma-c20.control.grid_protection.42087 |  | 42087 | 1 | U16 | 100 | Hz | RW | yes |
| ma-c20.control.grid_protection.42088 |  | 42088 | 1 | U16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42089 |  | 42089 | 1 | U16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42090 |  | 42090 | 1 | U16 | 100 | Hz | RW | yes |
| ma-c20.control.grid_protection.42091 |  | 42091 | 1 | U16 | 100 | Hz | RW | yes |
| ma-c20.control.grid_protection.42092 |  | 42092 | 1 | U16 | 1 | s | RW | yes |
| ma-c20.control.grid_protection.42097 |  | 42097 | 1 | U16 | 1000 | MΩ | RW | yes |
| ma-c20.control.grid_protection.42098 |  | 42098 | 1 | U16 | 10 | % | RW | yes |
| ma-c20.control.grid_protection.42099 |  | 42099 | 1 | U16 | 10 | ° | RW | yes |
| ma-c20.control.grid_protection.42100 |  | 42100 | 1 | U16 | 1 | s | RW | yes |
| ma-c20.control.grid_protection.42101 |  | 42101 | 1 | U16 | 1 | % | RW | yes |
| ma-c20.control.grid_protection.42102 |  | 42102 | 1 | U16 | 1 | % | RW | yes |
| ma-c20.control.grid_protection.42103 |  | 42103 | 1 | U16 | 1 | s | RW | yes |
| ma-c20.control.grid_protection.42104 |  | 42104 | 1 | U16 | 1 | s | RW | yes |
| ma-c20.control.grid_protection.42105 |  | 42105 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42107 |  | 42107 | 2 | U32 | 1000 | %/s | RW | yes |
| ma-c20.control.grid_protection.42109 |  | 42109 | 1 | U16 | 10 | % | RW | yes |
| ma-c20.control.grid_protection.42110 |  | 42110 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42111 |  | 42111 | 1 | U16 | 10 | % | RW | yes |
| ma-c20.control.grid_protection.42112 |  | 42112 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42113 |  | 42113 | 1 | U16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42114 |  | 42114 | 1 | U16 | 10 | not stated | RW | yes |
| ma-c20.control.grid_protection.42115 |  | 42115 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42116 |  | 42116 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42118 |  | 42118 | 1 | U16 | 100 | not stated | RW | yes |
| ma-c20.control.grid_protection.42119 |  | 42119 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42120 |  | 42120 | 1 | U16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42121 |  | 42121 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42122 |  | 42122 | 1 | U16 | 10 | not stated | RW | yes |
| ma-c20.control.grid_protection.42123 |  | 42123 | 1 | U16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42124 |  | 42124 | 1 | U16 | 1 | % | RW | yes |
| ma-c20.control.grid_protection.42125 |  | 42125 | 1 | U16 | 1 | %/s | RW | yes |
| ma-c20.control.grid_protection.42126 |  | 42126 | 1 | U16 | 10 | not stated | RW | yes |
| ma-c20.control.grid_protection.42127 |  | 42127 | 1 | U16 | 10 | not stated | RW | yes |
| ma-c20.control.grid_protection.42128 |  | 42128 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42129 |  | 42129 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42130 |  | 42130 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42131 |  | 42131 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42132 |  | 42132 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42133 |  | 42133 | 1 | U16 | 1 | % | RW | yes |
| ma-c20.control.grid_protection.42134 |  | 42134 | 1 | U16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42135 |  | 42135 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42136 |  | 42136 | 1 | U16 | 10 | mA | RW | yes |
| ma-c20.control.grid_protection.42138 |  | 42138 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42139 |  | 42139 | 1 | U16 | 10 | % | RW | yes |
| ma-c20.control.grid_protection.42140 |  | 42140 | 1 | U16 | 10 | % | RW | yes |
| ma-c20.control.grid_protection.42141 |  | 42141 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42142 |  | 42142 | 1 | U16 | 1 | % | RW | yes |
| ma-c20.control.grid_protection.42143 |  | 42143 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42144 |  | 42144 | 1 | U16 | 100 | Hz | RW | yes |
| ma-c20.control.grid_protection.42145 |  | 42145 | 1 | U16 | 1 | % | RW | yes |
| ma-c20.control.grid_protection.42146 |  | 42146 | 1 | U16 | 100 | Hz | RW | yes |
| ma-c20.control.grid_protection.42147 |  | 42147 | 1 | U16 | 100 | Hz | RW | yes |
| ma-c20.control.grid_protection.42148 |  | 42148 | 1 | U16 | 1 | %/min | RW | yes |
| ma-c20.control.grid_protection.42151 |  | 42151 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42152 |  | 42152 | 1 | U16 | 1 | %/min | RW | yes |
| ma-c20.control.grid_protection.42155 |  | 42155 | 21 | MLD | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42176 |  | 42176 | 1 | U16 | 100 | Hz | RW | yes |
| ma-c20.control.grid_protection.42177 |  | 42177 | 1 | U16 | 1 | % | RW | yes |
| ma-c20.control.grid_protection.42178 |  | 42178 | 1 | U16 | 100 | Hz | RW | yes |
| ma-c20.control.grid_protection.42179 |  | 42179 | 1 | U16 | 100 | Hz | RW | yes |
| ma-c20.control.grid_protection.42180 |  | 42180 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42181 |  | 42181 | 1 | I16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42182 |  | 42182 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42183 |  | 42183 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42192 |  | 42192 | 2 | U32 | 1000 | %/s | RW | yes |
| ma-c20.control.grid_protection.42200 |  | 42200 | 21 | MLD | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42221 |  | 42221 | 21 | MLD | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42242 |  | 42242 | 1 | U16 | 100 | s | RW | yes |
| ma-c20.control.grid_protection.42287 |  | 42287 | 1 | U16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42288 |  | 42288 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42290 |  | 42290 | 1 | U16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42291 |  | 42291 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42293 |  | 42293 | 1 | U16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42294 |  | 42294 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42296 |  | 42296 | 1 | U16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42297 |  | 42297 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42299 |  | 42299 | 1 | U16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42302 |  | 42302 | 1 | U16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42305 |  | 42305 | 1 | U16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42306 |  | 42306 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42308 |  | 42308 | 1 | U16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42309 |  | 42309 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42311 |  | 42311 | 1 | U16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42312 |  | 42312 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42314 |  | 42314 | 1 | U16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42315 |  | 42315 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42317 |  | 42317 | 1 | U16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42318 |  | 42318 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42320 |  | 42320 | 1 | U16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42321 |  | 42321 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42323 |  | 42323 | 1 | U16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42324 |  | 42324 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42326 |  | 42326 | 1 | U16 | 10 | V | RW | yes |
| ma-c20.control.grid_protection.42327 |  | 42327 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42329 |  | 42329 | 1 | U16 | 100 | Hz | RW | yes |
| ma-c20.control.grid_protection.42330 |  | 42330 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42332 |  | 42332 | 1 | U16 | 100 | Hz | RW | yes |
| ma-c20.control.grid_protection.42333 |  | 42333 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42335 |  | 42335 | 1 | U16 | 100 | Hz | RW | yes |
| ma-c20.control.grid_protection.42336 |  | 42336 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42338 |  | 42338 | 1 | U16 | 100 | Hz | RW | yes |
| ma-c20.control.grid_protection.42339 |  | 42339 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42341 |  | 42341 | 1 | U16 | 100 | Hz | RW | yes |
| ma-c20.control.grid_protection.42342 |  | 42342 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42344 |  | 42344 | 1 | U16 | 100 | Hz | RW | yes |
| ma-c20.control.grid_protection.42345 |  | 42345 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42347 |  | 42347 | 1 | U16 | 100 | Hz | RW | yes |
| ma-c20.control.grid_protection.42348 |  | 42348 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42350 |  | 42350 | 1 | U16 | 100 | Hz | RW | yes |
| ma-c20.control.grid_protection.42351 |  | 42351 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42353 |  | 42353 | 1 | U16 | 100 | Hz | RW | yes |
| ma-c20.control.grid_protection.42354 |  | 42354 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42356 |  | 42356 | 1 | U16 | 100 | Hz | RW | yes |
| ma-c20.control.grid_protection.42357 |  | 42357 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42359 |  | 42359 | 1 | U16 | 100 | Hz | RW | yes |
| ma-c20.control.grid_protection.42360 |  | 42360 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42362 |  | 42362 | 1 | U16 | 100 | Hz | RW | yes |
| ma-c20.control.grid_protection.42363 |  | 42363 | 2 | U32 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42400 |  | 42400 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42403 |  | 42403 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42404 |  | 42404 | 1 | U16 | 10 | Hz | RW | yes |
| ma-c20.control.grid_protection.42407 |  | 42407 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42409 |  | 42409 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42429 |  | 42429 | 1 | U16 | 1 | ms | RW | yes |
| ma-c20.control.grid_protection.42432 |  | 42432 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42590 |  | 42590 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42594 |  | 42594 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42595 |  | 42595 | 1 | U16 | 100 | not stated | RW | yes |
| ma-c20.control.grid_protection.42596 |  | 42596 | 1 | U16 | 1 | % | RW | yes |
| ma-c20.control.grid_protection.42597 |  | 42597 | 1 | I16 | 1 | min | RW | yes |
| ma-c20.control.grid_protection.42598 |  | 42598 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42599 |  | 42599 | 1 | E16 | not stated | not stated | RW | yes |
| ma-c20.control.grid_protection.42730 |  | 42730 | 1 | E16 | not stated | not stated | WO | yes |
| ma-c20.control.grid_protection.42779 |  | 42779 | 1 | E16 | not stated | not stated | WO | yes |

## MA-V200

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| metadata.grid_code_protocol_id.30109 |  | 30109 | 1 | U16 | not stated | not stated | RO | no |
| capability.grid_mask_1.30219 |  | 30219 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_2.30220 |  | 30220 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_3.30221 |  | 30221 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_4.30222 |  | 30222 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_5.30223 |  | 30223 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_6.30224 |  | 30224 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_7.30225 |  | 30225 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_8.30226 |  | 30226 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_9.30227 |  | 30227 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_10.30228 |  | 30228 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_11.30229 |  | 30229 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_12.30230 |  | 30230 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_13.30231 |  | 30231 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_14.30232 |  | 30232 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_15.30233 |  | 30233 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_16.30234 |  | 30234 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_17.30235 |  | 30235 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_18.30236 |  | 30236 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_19.30237 |  | 30237 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_20.30238 |  | 30238 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_21.30239 |  | 30239 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_22.30240 |  | 30240 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_23.30241 |  | 30241 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_24.30242 |  | 30242 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_25.30243 |  | 30243 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_26.30244 |  | 30244 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_27.30245 |  | 30245 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_28.30246 |  | 30246 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_29.30247 |  | 30247 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_30.30248 |  | 30248 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_31.30249 |  | 30249 | 1 | E16 | not stated | not stated | RO | no |
| capability.grid_mask_32.30250 |  | 30250 | 1 | E16 | not stated | not stated | RO | no |
| ma-v200.control.grid_code |  | 42000 | 1 | U16 | 1 | not stated | RW | no |
| ma-v200.control.output_mode |  | 42001 | 1 | E16 | not stated | not stated | RW | no |
| ma-v200.control.nominal_voltage |  | 42002 | 1 | U16 | 1 | V | RW | no |
| ma-v200.control.nominal_frequency |  | 42003 | 1 | U16 | 1 | Hz | RW | no |
| ma-v200.control.isolation_mode |  | 42050 | 1 | E16 | not stated | not stated | RW | no |
| ma-v200.control.power_factor_target |  | 40122 | 1 | I16 | 1000 | not stated | RW | no |
| ma-v200.control.reactive_ratio_target |  | 40123 | 1 | I16 | 1000 | not stated | RW | no |
| ma-v200.control.reactive_adjustment_time |  | 40124 | 1 | U16 | 1 | s | RW | no |
| ma-v200.control.night_reactive_ratio |  | 40128 | 1 | I16 | 1000 | not stated | RW | no |
| ma-v200.control.night_reactive_power |  | 40129 | 2 | I32 | 1000 | kVar | RW | no |
| ma-v200.control.curve_reactive_adjustment_time |  | 40196 | 1 | U16 | 1 | s | RW | no |
| ma-v200.control.qu_mode |  | 40037 | 1 | E16 | not stated | not stated | RW | no |
| ma-v200.control.qu_trigger_power |  | 40038 | 1 | I16 | 1 | % | RW | no |
| ma-v200.control.qu_exit_power |  | 40198 | 1 | I16 | 1 | % | RW | no |
| ma-v200.control.qu_minimum_power_factor |  | 40375 | 1 | U16 | 1000 | not stated | RW | no |
| ma-v200.control.qu_activation_delay |  | 40376 | 1 | U16 | 1 | s | RW | no |
| ma-v200.control.curve_aux_42032 |  | 42032 | 1 | U16 | 10 | s | RW | yes |
| ma-v200.control.curve_aux_42101 |  | 42101 | 1 | U16 | 1 | % | RW | yes |
| ma-v200.control.curve_aux_42102 |  | 42102 | 1 | U16 | 1 | % | RW | yes |
| ma-v200.control.pu_adjustment_time |  | 42242 | 1 | U16 | 100 | s | RW | no |
| ma-v200.control.cosphi_pn_curve |  | 40133 | 21 | MLD | not stated | not stated | RW | no |
| ma-v200.control.qu_characteristic_curve |  | 40154 | 21 | MLD | not stated | not stated | RW | no |
| ma-v200.control.pfu_characteristic_curve |  | 40175 | 21 | MLD | not stated | not stated | RW | no |
| ma-v200.control.qp_curve |  | 40354 | 21 | MLD | not stated | not stated | RW | no |
| ma-v200.control.lvrt_curve |  | 42155 | 21 | MLD | not stated | not stated | RW | no |
| ma-v200.control.pu_curve |  | 42221 | 21 | MLD | not stated | not stated | RW | no |
| ma-v200.control.distributed_active_percent |  | 40199 | 1 | I16 | 10 | % | RW | no |
| ma-v200.control.frequency_power_42040 |  | 42040 | 1 | U16 | 1 | s | RW | yes |
| ma-v200.control.frequency_power_42041 |  | 42041 | 1 | U16 | 1 | ms | RW | yes |
| ma-v200.control.frequency_power_42042 |  | 42042 | 1 | E16 | not stated | not stated | RW | yes |
| ma-v200.control.frequency_power_42043 |  | 42043 | 1 | U16 | 1000 | Hz | RW | yes |
| ma-v200.control.frequency_power_42051 |  | 42051 | 1 | U16 | 1 | %/min | RW | yes |
| ma-v200.control.frequency_power_42052 |  | 42052 | 1 | U16 | 10 | % | RW | yes |
| ma-v200.control.frequency_power_42053 |  | 42053 | 1 | U16 | 1 | ms | RW | no |
| ma-v200.control.frequency_power_42141 |  | 42141 | 1 | E16 | not stated | not stated | RW | no |
| ma-v200.control.frequency_power_42142 |  | 42142 | 1 | U16 | 1 | % | RW | no |
| ma-v200.control.frequency_power_42143 |  | 42143 | 1 | E16 | not stated | not stated | RW | no |
| ma-v200.control.frequency_power_42144 |  | 42144 | 1 | U16 | 100 | Hz | RW | no |
| ma-v200.control.frequency_power_42145 |  | 42145 | 1 | U16 | 1 | % | RW | no |
| ma-v200.control.frequency_power_42146 |  | 42146 | 1 | U16 | 100 | Hz | RW | no |
| ma-v200.control.frequency_power_42147 |  | 42147 | 1 | U16 | 100 | Hz | RW | no |
| ma-v200.control.frequency_power_42148 |  | 42148 | 1 | U16 | 1 | %/min | RW | no |
| ma-v200.control.frequency_power_42151 |  | 42151 | 1 | E16 | not stated | not stated | RW | no |
| ma-v200.control.frequency_power_42152 |  | 42152 | 1 | U16 | 1 | %/min | RW | no |
| ma-v200.control.frequency_power_42176 |  | 42176 | 1 | U16 | 100 | Hz | RW | no |
| ma-v200.control.frequency_power_42177 |  | 42177 | 1 | U16 | 1 | % | RW | no |
| ma-v200.control.frequency_power_42178 |  | 42178 | 1 | U16 | 100 | Hz | RW | no |
| ma-v200.control.frequency_power_42179 |  | 42179 | 1 | U16 | 100 | Hz | RW | no |
| ma-v200.control.frequency_slope_42080 |  | 42080 | 1 | E16 | not stated | not stated | RW | yes |
| ma-v200.control.frequency_slope_42081 |  | 42081 | 1 | U16 | 10 | Hz/s | RW | yes |
| ma-v200.control.frequency_slope_42082 |  | 42082 | 1 | U16 | 10 | s | RW | yes |
| ma-v200.control.ten_minute_overvoltage_threshold |  | 42290 | 1 | U16 | 10 | V | RW | no |
| ma-v200.control.ten_minute_overvoltage_duration |  | 42291 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.overvoltage_level_1_threshold |  | 42293 | 1 | U16 | 10 | V | RW | no |
| ma-v200.control.overvoltage_level_1_duration |  | 42294 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.overvoltage_level_2_threshold |  | 42296 | 1 | U16 | 10 | V | RW | no |
| ma-v200.control.overvoltage_level_2_duration |  | 42297 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.overvoltage_level_3_threshold |  | 42299 | 1 | U16 | 10 | V | RW | no |
| ma-v200.control.overvoltage_level_3_duration |  | 42300 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.overvoltage_level_4_threshold |  | 42302 | 1 | U16 | 10 | V | RW | no |
| ma-v200.control.overvoltage_level_4_duration |  | 42303 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.overvoltage_level_5_threshold |  | 42305 | 1 | U16 | 10 | V | RW | no |
| ma-v200.control.overvoltage_level_5_duration |  | 42306 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.overvoltage_level_6_threshold |  | 42308 | 1 | U16 | 10 | V | RW | no |
| ma-v200.control.overvoltage_level_6_duration |  | 42309 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.undervoltage_level_1_threshold |  | 42311 | 1 | U16 | 10 | V | RW | no |
| ma-v200.control.undervoltage_level_1_duration |  | 42312 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.undervoltage_level_2_threshold |  | 42314 | 1 | U16 | 10 | V | RW | no |
| ma-v200.control.undervoltage_level_2_duration |  | 42315 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.undervoltage_level_3_threshold |  | 42317 | 1 | U16 | 10 | V | RW | no |
| ma-v200.control.undervoltage_level_3_duration |  | 42318 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.undervoltage_level_4_threshold |  | 42320 | 1 | U16 | 10 | V | RW | no |
| ma-v200.control.undervoltage_level_4_duration |  | 42321 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.undervoltage_level_5_threshold |  | 42323 | 1 | U16 | 10 | V | RW | no |
| ma-v200.control.undervoltage_level_5_duration |  | 42324 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.undervoltage_level_6_threshold |  | 42326 | 1 | U16 | 10 | V | RW | no |
| ma-v200.control.undervoltage_level_6_duration |  | 42327 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.overfrequency_level_1_threshold |  | 42329 | 1 | U16 | 100 | Hz | RW | no |
| ma-v200.control.overfrequency_level_1_duration |  | 42330 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.overfrequency_level_2_threshold |  | 42332 | 1 | U16 | 100 | Hz | RW | no |
| ma-v200.control.overfrequency_level_2_duration |  | 42333 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.overfrequency_level_3_threshold |  | 42335 | 1 | U16 | 100 | Hz | RW | no |
| ma-v200.control.overfrequency_level_3_duration |  | 42336 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.overfrequency_level_4_threshold |  | 42338 | 1 | U16 | 100 | Hz | RW | no |
| ma-v200.control.overfrequency_level_4_duration |  | 42339 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.overfrequency_level_5_threshold |  | 42341 | 1 | U16 | 100 | Hz | RW | no |
| ma-v200.control.overfrequency_level_5_duration |  | 42342 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.overfrequency_level_6_threshold |  | 42344 | 1 | U16 | 100 | Hz | RW | no |
| ma-v200.control.overfrequency_level_6_duration |  | 42345 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.underfrequency_level_1_threshold |  | 42347 | 1 | U16 | 100 | Hz | RW | no |
| ma-v200.control.underfrequency_level_1_duration |  | 42348 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.underfrequency_level_2_threshold |  | 42350 | 1 | U16 | 100 | Hz | RW | no |
| ma-v200.control.underfrequency_level_2_duration |  | 42351 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.underfrequency_level_3_threshold |  | 42353 | 1 | U16 | 100 | Hz | RW | no |
| ma-v200.control.underfrequency_level_3_duration |  | 42354 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.underfrequency_level_4_threshold |  | 42356 | 1 | U16 | 100 | Hz | RW | no |
| ma-v200.control.underfrequency_level_4_duration |  | 42357 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.underfrequency_level_5_threshold |  | 42359 | 1 | U16 | 100 | Hz | RW | no |
| ma-v200.control.underfrequency_level_5_duration |  | 42360 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.underfrequency_level_6_threshold |  | 42362 | 1 | U16 | 100 | Hz | RW | no |
| ma-v200.control.underfrequency_level_6_duration |  | 42363 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.ramps_42100 |  | 42100 | 1 | U16 | 1 | s | RW | no |
| ma-v200.control.ride_through_42110 |  | 42110 | 1 | E16 | not stated | not stated | RW | yes |
| ma-v200.control.grid_jump_threshold |  | 42111 | 1 | U16 | 10 | % | RW | no |
| ma-v200.control.ride_through_42112 |  | 42112 | 1 | E16 | not stated | not stated | RW | yes |
| ma-v200.control.ride_through_42113 |  | 42113 | 1 | U16 | 10 | V | RW | yes |
| ma-v200.control.ride_through_42114 |  | 42114 | 1 | U16 | 10 | not stated | RW | yes |
| ma-v200.control.ride_through_42118 |  | 42118 | 1 | U16 | 100 | not stated | RW | yes |
| ma-v200.control.ride_through_42119 |  | 42119 | 1 | E16 | not stated | not stated | RW | no |
| ma-v200.control.ride_through_42120 |  | 42120 | 1 | U16 | 10 | V | RW | no |
| ma-v200.control.ride_through_42121 |  | 42121 | 1 | E16 | not stated | not stated | RW | no |
| ma-v200.control.ride_through_42122 |  | 42122 | 1 | U16 | 10 | not stated | RW | no |
| ma-v200.control.vrt_exit_hysteresis |  | 42123 | 1 | U16 | 10 | V | RW | no |
| ma-v200.control.ride_through_42124 |  | 42124 | 1 | U16 | 1 | % | RW | yes |
| ma-v200.control.ride_through_42125 |  | 42125 | 1 | U16 | 1 | %/s | RW | yes |
| ma-v200.control.ride_through_42126 |  | 42126 | 1 | U16 | 10 | not stated | RW | yes |
| ma-v200.control.ride_through_42127 |  | 42127 | 1 | U16 | 10 | not stated | RW | no |
| ma-v200.control.ride_through_42128 |  | 42128 | 1 | E16 | not stated | not stated | RW | yes |
| ma-v200.control.ride_through_42133 |  | 42133 | 1 | U16 | 1 | % | RW | no |
| ma-v200.control.lvrt_zero_current_threshold_conflict |  | 42134 | 1 | U16 | 10 | V | RW | no |
| ma-v200.control.ride_through_42135 |  | 42135 | 1 | E16 | not stated | not stated | RW | yes |
| ma-v200.control.voltage_rise_42138 |  | 42138 | 1 | E16 | not stated | not stated | RW | yes |
| ma-v200.control.voltage_rise_reactive_point |  | 42139 | 1 | U16 | 10 | % | RW | no |
| ma-v200.control.voltage_rise_active_point |  | 42140 | 1 | U16 | 10 | % | RW | no |
| ma-v200.control.islanding_42129 |  | 42129 | 1 | E16 | not stated | not stated | RW | yes |
| ma-v200.control.islanding_42130 |  | 42130 | 1 | E16 | not stated | not stated | RW | yes |
| ma-v200.control.islanding_42131 |  | 42131 | 1 | E16 | not stated | not stated | RW | yes |
| ma-v200.control.normal_connection_42084 |  | 42084 | 1 | U16 | 10 | V | RW | no |
| ma-v200.control.normal_connection_42085 |  | 42085 | 1 | U16 | 10 | V | RW | no |
| ma-v200.control.normal_connection_42086 |  | 42086 | 1 | U16 | 100 | Hz | RW | no |
| ma-v200.control.normal_connection_42087 |  | 42087 | 1 | U16 | 100 | Hz | RW | no |
| ma-v200.control.startup_recovery_42088 |  | 42088 | 1 | U16 | 10 | V | RW | no |
| ma-v200.control.startup_recovery_42089 |  | 42089 | 1 | U16 | 10 | V | RW | no |
| ma-v200.control.startup_recovery_42090 |  | 42090 | 1 | U16 | 100 | Hz | RW | no |
| ma-v200.control.startup_recovery_42091 |  | 42091 | 1 | U16 | 100 | Hz | RW | no |
| ma-v200.control.startup_recovery_42105 |  | 42105 | 2 | U32 | 1 | ms | RW | no |
| ma-v200.control.rapid_restart_mode |  | 42116 | 1 | E16 | not stated | not stated | RW | no |

## MB02

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| mb02.grid_code_protocol_id |  | 30109 | 1 | U16 | not stated | not stated | RO | no |
| mb02.control.grid_code |  | 42000 | 1 | U16 | 1 | not stated | RW | no |
| mb02.control.output_mode |  | 42001 | 1 | E16 | not stated | not stated | RW | no |
| mb02.control.nominal_voltage |  | 42002 | 1 | U16 | 1 | V | RW | no |
| mb02.control.nominal_frequency |  | 42003 | 1 | U16 | 1 | Hz | RW | no |
| mb02.control.isolation_mode |  | 42050 | 1 | E16 | not stated | not stated | RW | no |
| mb02.control.power_factor_target |  | 40122 | 1 | I16 | 1000 | not stated | RW | no |
| mb02.control.reactive_ratio_target |  | 40123 | 1 | I16 | 1000 | not stated | RW | no |
| mb02.control.reactive_adjustment_time |  | 40124 | 1 | U16 | 1 | s | RW | no |
| mb02.control.night_reactive_ratio |  | 40128 | 1 | I16 | 1000 | not stated | RW | no |
| mb02.control.night_reactive_power |  | 40129 | 2 | I32 | 1000 | kVar | RW | no |
| mb02.control.curve_reactive_adjustment_time |  | 40196 | 1 | U16 | 1 | s | RW | no |
| mb02.control.qu_mode |  | 40037 | 1 | E16 | not stated | not stated | RW | no |
| mb02.control.qu_trigger_power |  | 40038 | 1 | I16 | 1 | % | RW | no |
| mb02.control.qu_exit_power |  | 40198 | 1 | I16 | 1 | % | RW | no |
| mb02.control.qu_minimum_power_factor |  | 40375 | 1 | U16 | 1000 | not stated | RW | no |
| mb02.control.qu_activation_delay |  | 40376 | 1 | U16 | 1 | s | RW | no |
| mb02.control.curve_aux_42032 |  | 42032 | 1 | U16 | 10 | s | RW | yes |
| mb02.control.curve_aux_42101 |  | 42101 | 1 | U16 | 1 | % | RW | yes |
| mb02.control.curve_aux_42102 |  | 42102 | 1 | U16 | 1 | % | RW | yes |
| mb02.control.pu_adjustment_time |  | 42242 | 1 | U16 | 100 | s | RW | no |
| mb02.control.cosphi_pn_curve |  | 40133 | 21 | MLD | not stated | not stated | RW | no |
| mb02.control.qu_characteristic_curve |  | 40154 | 21 | MLD | not stated | not stated | RW | no |
| mb02.control.pfu_characteristic_curve |  | 40175 | 21 | MLD | not stated | not stated | RW | no |
| mb02.control.qp_curve |  | 40354 | 21 | MLD | not stated | not stated | RW | no |
| mb02.control.lvrt_curve |  | 42155 | 21 | MLD | not stated | not stated | RW | no |
| mb02.control.qu_curve_alternate |  | 42200 | 21 | MLD | not stated | not stated | RW | no |
| mb02.control.pu_curve |  | 42221 | 21 | MLD | not stated | not stated | RW | no |
| mb02.control.distributed_active_percent |  | 40199 | 1 | I16 | 10 | % | RW | no |
| mb02.control.frequency_power_42040 |  | 42040 | 1 | U16 | 1 | s | RW | yes |
| mb02.control.frequency_power_42041 |  | 42041 | 1 | U16 | 1 | ms | RW | yes |
| mb02.control.frequency_power_42042 |  | 42042 | 1 | E16 | not stated | not stated | RW | yes |
| mb02.control.frequency_power_42043 |  | 42043 | 1 | U16 | 1000 | Hz | RW | yes |
| mb02.control.frequency_power_42051 |  | 42051 | 1 | U16 | 1 | %/min | RW | yes |
| mb02.control.frequency_power_42052 |  | 42052 | 1 | U16 | 10 | % | RW | yes |
| mb02.control.frequency_power_42053 |  | 42053 | 1 | U16 | 1 | ms | RW | no |
| mb02.control.frequency_power_42141 |  | 42141 | 1 | E16 | not stated | not stated | RW | no |
| mb02.control.frequency_power_42142 |  | 42142 | 1 | U16 | 1 | % | RW | no |
| mb02.control.frequency_power_42143 |  | 42143 | 1 | E16 | not stated | not stated | RW | no |
| mb02.control.frequency_power_42144 |  | 42144 | 1 | U16 | 100 | Hz | RW | no |
| mb02.control.frequency_power_42145 |  | 42145 | 1 | U16 | 1 | % | RW | no |
| mb02.control.frequency_power_42146 |  | 42146 | 1 | U16 | 100 | Hz | RW | no |
| mb02.control.frequency_power_42147 |  | 42147 | 1 | U16 | 100 | Hz | RW | no |
| mb02.control.frequency_power_42148 |  | 42148 | 1 | U16 | 1 | %/min | RW | no |
| mb02.control.frequency_power_42151 |  | 42151 | 1 | E16 | not stated | not stated | RW | no |
| mb02.control.frequency_power_42152 |  | 42152 | 1 | U16 | 1 | %/min | RW | no |
| mb02.control.frequency_power_42176 |  | 42176 | 1 | U16 | 100 | Hz | RW | no |
| mb02.control.frequency_power_42177 |  | 42177 | 1 | U16 | 1 | % | RW | no |
| mb02.control.frequency_power_42178 |  | 42178 | 1 | U16 | 100 | Hz | RW | no |
| mb02.control.frequency_power_42179 |  | 42179 | 1 | U16 | 100 | Hz | RW | no |
| mb02.control.frequency_slope_42080 |  | 42080 | 1 | E16 | not stated | not stated | RW | yes |
| mb02.control.frequency_slope_42081 |  | 42081 | 1 | U16 | 10 | Hz/s | RW | yes |
| mb02.control.frequency_slope_42082 |  | 42082 | 1 | U16 | 10 | s | RW | yes |
| mb02.control.ten_minute_undervoltage_threshold |  | 42287 | 1 | U16 | 10 | V | RW | no |
| mb02.control.ten_minute_undervoltage_duration |  | 42288 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.ten_minute_overvoltage_threshold |  | 42290 | 1 | U16 | 10 | V | RW | no |
| mb02.control.ten_minute_overvoltage_duration |  | 42291 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.overvoltage_level_1_threshold |  | 42293 | 1 | U16 | 10 | V | RW | no |
| mb02.control.overvoltage_level_1_duration |  | 42294 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.overvoltage_level_2_threshold |  | 42296 | 1 | U16 | 10 | V | RW | no |
| mb02.control.overvoltage_level_2_duration |  | 42297 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.overvoltage_level_3_threshold |  | 42299 | 1 | U16 | 10 | V | RW | no |
| mb02.control.overvoltage_level_3_duration |  | 42300 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.overvoltage_level_4_threshold |  | 42302 | 1 | U16 | 10 | V | RW | no |
| mb02.control.overvoltage_level_4_duration |  | 42303 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.overvoltage_level_5_threshold |  | 42305 | 1 | U16 | 10 | V | RW | no |
| mb02.control.overvoltage_level_5_duration |  | 42306 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.overvoltage_level_6_threshold |  | 42308 | 1 | U16 | 10 | V | RW | no |
| mb02.control.overvoltage_level_6_duration |  | 42309 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.undervoltage_level_1_threshold |  | 42311 | 1 | U16 | 10 | V | RW | no |
| mb02.control.undervoltage_level_1_duration |  | 42312 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.undervoltage_level_2_threshold |  | 42314 | 1 | U16 | 10 | V | RW | no |
| mb02.control.undervoltage_level_2_duration |  | 42315 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.undervoltage_level_3_threshold |  | 42317 | 1 | U16 | 10 | V | RW | no |
| mb02.control.undervoltage_level_3_duration |  | 42318 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.undervoltage_level_4_threshold |  | 42320 | 1 | U16 | 10 | V | RW | no |
| mb02.control.undervoltage_level_4_duration |  | 42321 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.undervoltage_level_5_threshold |  | 42323 | 1 | U16 | 10 | V | RW | no |
| mb02.control.undervoltage_level_5_duration |  | 42324 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.undervoltage_level_6_threshold |  | 42326 | 1 | U16 | 10 | V | RW | no |
| mb02.control.undervoltage_level_6_duration |  | 42327 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.overfrequency_level_1_threshold |  | 42329 | 1 | U16 | 100 | Hz | RW | no |
| mb02.control.overfrequency_level_1_duration |  | 42330 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.overfrequency_level_2_threshold |  | 42332 | 1 | U16 | 100 | Hz | RW | no |
| mb02.control.overfrequency_level_2_duration |  | 42333 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.overfrequency_level_3_threshold |  | 42335 | 1 | U16 | 100 | Hz | RW | no |
| mb02.control.overfrequency_level_3_duration |  | 42336 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.overfrequency_level_4_threshold |  | 42338 | 1 | U16 | 100 | Hz | RW | no |
| mb02.control.overfrequency_level_4_duration |  | 42339 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.overfrequency_level_5_threshold |  | 42341 | 1 | U16 | 100 | Hz | RW | no |
| mb02.control.overfrequency_level_5_duration |  | 42342 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.overfrequency_level_6_threshold |  | 42344 | 1 | U16 | 100 | Hz | RW | no |
| mb02.control.overfrequency_level_6_duration |  | 42345 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.underfrequency_level_1_threshold |  | 42347 | 1 | U16 | 100 | Hz | RW | no |
| mb02.control.underfrequency_level_1_duration |  | 42348 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.underfrequency_level_2_threshold |  | 42350 | 1 | U16 | 100 | Hz | RW | no |
| mb02.control.underfrequency_level_2_duration |  | 42351 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.underfrequency_level_3_threshold |  | 42353 | 1 | U16 | 100 | Hz | RW | no |
| mb02.control.underfrequency_level_3_duration |  | 42354 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.underfrequency_level_4_threshold |  | 42356 | 1 | U16 | 100 | Hz | RW | no |
| mb02.control.underfrequency_level_4_duration |  | 42357 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.underfrequency_level_5_threshold |  | 42359 | 1 | U16 | 100 | Hz | RW | no |
| mb02.control.underfrequency_level_5_duration |  | 42360 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.underfrequency_level_6_threshold |  | 42362 | 1 | U16 | 100 | Hz | RW | no |
| mb02.control.underfrequency_level_6_duration |  | 42363 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.ramps_42100 |  | 42100 | 1 | U16 | 1 | s | RW | no |
| mb02.control.ride_through_42110 |  | 42110 | 1 | E16 | not stated | not stated | RW | yes |
| mb02.control.grid_jump_threshold |  | 42111 | 1 | U16 | 10 | % | RW | no |
| mb02.control.ride_through_42112 |  | 42112 | 1 | E16 | not stated | not stated | RW | yes |
| mb02.control.ride_through_42113 |  | 42113 | 1 | U16 | 10 | V | RW | yes |
| mb02.control.ride_through_42114 |  | 42114 | 1 | U16 | 10 | not stated | RW | yes |
| mb02.control.ride_through_42118 |  | 42118 | 1 | U16 | 100 | not stated | RW | yes |
| mb02.control.ride_through_42119 |  | 42119 | 1 | E16 | not stated | not stated | RW | no |
| mb02.control.ride_through_42120 |  | 42120 | 1 | U16 | 10 | V | RW | no |
| mb02.control.ride_through_42121 |  | 42121 | 1 | E16 | not stated | not stated | RW | no |
| mb02.control.ride_through_42122 |  | 42122 | 1 | U16 | 10 | not stated | RW | no |
| mb02.control.vrt_exit_hysteresis |  | 42123 | 1 | U16 | 10 | V | RW | no |
| mb02.control.ride_through_42124 |  | 42124 | 1 | U16 | 1 | % | RW | yes |
| mb02.control.ride_through_42125 |  | 42125 | 1 | U16 | 1 | %/s | RW | yes |
| mb02.control.ride_through_42126 |  | 42126 | 1 | U16 | 10 | not stated | RW | yes |
| mb02.control.ride_through_42127 |  | 42127 | 1 | U16 | 10 | not stated | RW | no |
| mb02.control.ride_through_42128 |  | 42128 | 1 | E16 | not stated | not stated | RW | yes |
| mb02.control.ride_through_42133 |  | 42133 | 1 | U16 | 1 | % | RW | no |
| mb02.control.lvrt_zero_current_threshold_conflict |  | 42134 | 1 | U16 | 10 | V | RW | no |
| mb02.control.ride_through_42135 |  | 42135 | 1 | E16 | not stated | not stated | RW | yes |
| mb02.control.voltage_rise_42138 |  | 42138 | 1 | E16 | not stated | not stated | RW | yes |
| mb02.control.voltage_rise_reactive_point |  | 42139 | 1 | U16 | 10 | % | RW | no |
| mb02.control.voltage_rise_active_point |  | 42140 | 1 | U16 | 10 | % | RW | no |
| mb02.control.islanding_42129 |  | 42129 | 1 | E16 | not stated | not stated | RW | yes |
| mb02.control.islanding_42130 |  | 42130 | 1 | E16 | not stated | not stated | RW | yes |
| mb02.control.islanding_42131 |  | 42131 | 1 | E16 | not stated | not stated | RW | yes |
| mb02.control.normal_connection_42084 |  | 42084 | 1 | U16 | 10 | V | RW | no |
| mb02.control.normal_connection_42085 |  | 42085 | 1 | U16 | 10 | V | RW | no |
| mb02.control.normal_connection_42086 |  | 42086 | 1 | U16 | 100 | Hz | RW | no |
| mb02.control.normal_connection_42087 |  | 42087 | 1 | U16 | 100 | Hz | RW | no |
| mb02.control.startup_recovery_42088 |  | 42088 | 1 | U16 | 10 | V | RW | no |
| mb02.control.startup_recovery_42089 |  | 42089 | 1 | U16 | 10 | V | RW | no |
| mb02.control.startup_recovery_42090 |  | 42090 | 1 | U16 | 100 | Hz | RW | no |
| mb02.control.startup_recovery_42091 |  | 42091 | 1 | U16 | 100 | Hz | RW | no |
| mb02.control.startup_recovery_42105 |  | 42105 | 2 | U32 | 1 | ms | RW | no |
| mb02.control.rapid_restart_mode |  | 42116 | 1 | E16 | not stated | not stated | RW | no |

## MC

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| metadata.grid_code_protocol_id.30109 |  | 30109 | 1 | U16 | not stated | not stated | RO | no |
| mc.control.grid_code |  | 42000 | 1 | U16 | 1 | not stated | RW | no |
| mc.control.output_mode |  | 42001 | 1 | E16 | not stated | not stated | RW | no |
| mc.control.nominal_voltage |  | 42002 | 1 | U16 | 1 | V | RW | no |
| mc.control.nominal_frequency |  | 42003 | 1 | U16 | 1 | Hz | RW | no |
| mc.control.isolation_mode |  | 42050 | 1 | E16 | not stated | not stated | RW | no |
| mc.control.power_factor_target |  | 40122 | 1 | I16 | 1000 | not stated | RW | no |
| mc.control.reactive_ratio_target |  | 40123 | 1 | I16 | 1000 | not stated | RW | no |
| mc.control.reactive_adjustment_time |  | 40124 | 1 | U16 | 1 | s | RW | no |
| mc.control.night_reactive_ratio |  | 40128 | 1 | I16 | 1000 | not stated | RW | no |
| mc.control.night_reactive_power |  | 40129 | 2 | I32 | 1000 | kVar | RW | no |
| mc.control.curve_reactive_adjustment_time |  | 40196 | 1 | U16 | 1 | s | RW | no |
| mc.control.qu_mode |  | 40037 | 1 | E16 | not stated | not stated | RW | no |
| mc.control.qu_trigger_power |  | 40038 | 1 | I16 | 1 | % | RW | no |
| mc.control.qu_exit_power |  | 40198 | 1 | I16 | 1 | % | RW | no |
| mc.control.qu_minimum_power_factor |  | 40375 | 1 | U16 | 1000 | not stated | RW | no |
| mc.control.qu_activation_delay |  | 40376 | 1 | U16 | 1 | s | RW | no |
| mc.control.curve_aux_42032 |  | 42032 | 1 | U16 | 10 | s | RW | yes |
| mc.control.curve_aux_42101 |  | 42101 | 1 | U16 | 1 | % | RW | yes |
| mc.control.curve_aux_42102 |  | 42102 | 1 | U16 | 1 | % | RW | yes |
| mc.control.pu_adjustment_time |  | 42242 | 1 | U16 | 100 | s | RW | no |
| mc.control.cosphi_pn_curve |  | 40133 | 21 | MLD | not stated | not stated | RW | no |
| mc.control.qu_characteristic_curve |  | 40154 | 21 | MLD | not stated | not stated | RW | no |
| mc.control.pfu_characteristic_curve |  | 40175 | 21 | MLD | not stated | not stated | RW | no |
| mc.control.qp_curve |  | 40354 | 21 | MLD | not stated | not stated | RW | no |
| mc.control.lvrt_curve |  | 42155 | 21 | MLD | not stated | not stated | RW | no |
| mc.control.qu_curve_alternate |  | 42200 | 21 | MLD | not stated | not stated | RW | no |
| mc.control.pu_curve |  | 42221 | 21 | MLD | not stated | not stated | RW | no |
| mc.control.distributed_active_percent |  | 40199 | 1 | I16 | 10 | % | RW | no |
| mc.control.frequency_power_42040 |  | 42040 | 1 | U16 | 1 | s | RW | yes |
| mc.control.frequency_power_42041 |  | 42041 | 1 | U16 | 1 | ms | RW | yes |
| mc.control.frequency_power_42042 |  | 42042 | 1 | E16 | not stated | not stated | RW | yes |
| mc.control.frequency_power_42043 |  | 42043 | 1 | U16 | 1000 | Hz | RW | yes |
| mc.control.frequency_power_42051 |  | 42051 | 1 | U16 | 1 | %/min | RW | yes |
| mc.control.frequency_power_42052 |  | 42052 | 1 | U16 | 10 | % | RW | yes |
| mc.control.frequency_power_42053 |  | 42053 | 1 | U16 | 1 | ms | RW | no |
| mc.control.frequency_power_42141 |  | 42141 | 1 | E16 | not stated | not stated | RW | no |
| mc.control.frequency_power_42142 |  | 42142 | 1 | U16 | 1 | % | RW | no |
| mc.control.frequency_power_42143 |  | 42143 | 1 | E16 | not stated | not stated | RW | no |
| mc.control.frequency_power_42144 |  | 42144 | 1 | U16 | 100 | Hz | RW | no |
| mc.control.frequency_power_42145 |  | 42145 | 1 | U16 | 1 | % | RW | no |
| mc.control.frequency_power_42146 |  | 42146 | 1 | U16 | 100 | Hz | RW | no |
| mc.control.frequency_power_42147 |  | 42147 | 1 | U16 | 100 | Hz | RW | no |
| mc.control.frequency_power_42148 |  | 42148 | 1 | U16 | 1 | %/min | RW | no |
| mc.control.frequency_power_42151 |  | 42151 | 1 | E16 | not stated | not stated | RW | no |
| mc.control.frequency_power_42152 |  | 42152 | 1 | U16 | 1 | %/min | RW | no |
| mc.control.frequency_power_42176 |  | 42176 | 1 | U16 | 100 | Hz | RW | no |
| mc.control.frequency_power_42177 |  | 42177 | 1 | U16 | 1 | % | RW | no |
| mc.control.frequency_power_42178 |  | 42178 | 1 | U16 | 100 | Hz | RW | no |
| mc.control.frequency_power_42179 |  | 42179 | 1 | U16 | 100 | Hz | RW | no |
| mc.control.frequency_power.field_42429 |  | 42429 | 1 | U16 | 1 | ms | RW | no |
| mc.control.frequency_slope_42080 |  | 42080 | 1 | E16 | not stated | not stated | RW | yes |
| mc.control.frequency_slope_42081 |  | 42081 | 1 | U16 | 10 | Hz/s | RW | yes |
| mc.control.frequency_slope_42082 |  | 42082 | 1 | U16 | 10 | s | RW | yes |
| mc.control.ten_minute_undervoltage_threshold |  | 42287 | 1 | U16 | 10 | V | RW | no |
| mc.control.ten_minute_undervoltage_duration |  | 42288 | 2 | U32 | 1 | ms | RW | no |
| mc.control.ten_minute_overvoltage_threshold |  | 42290 | 1 | U16 | 10 | V | RW | no |
| mc.control.ten_minute_overvoltage_duration |  | 42291 | 2 | U32 | 1 | ms | RW | no |
| mc.control.overvoltage_level_1_threshold |  | 42293 | 1 | U16 | 10 | V | RW | no |
| mc.control.overvoltage_level_1_duration |  | 42294 | 2 | U32 | 1 | ms | RW | no |
| mc.control.overvoltage_level_2_threshold |  | 42296 | 1 | U16 | 10 | V | RW | no |
| mc.control.overvoltage_level_2_duration |  | 42297 | 2 | U32 | 1 | ms | RW | no |
| mc.control.overvoltage_level_3_threshold |  | 42299 | 1 | U16 | 10 | V | RW | no |
| mc.control.overvoltage_level_3_duration |  | 42300 | 2 | U32 | 1 | ms | RW | no |
| mc.control.overvoltage_level_4_threshold |  | 42302 | 1 | U16 | 10 | V | RW | no |
| mc.control.overvoltage_level_4_duration |  | 42303 | 2 | U32 | 1 | ms | RW | no |
| mc.control.overvoltage_level_5_threshold |  | 42305 | 1 | U16 | 10 | V | RW | no |
| mc.control.overvoltage_level_5_duration |  | 42306 | 2 | U32 | 1 | ms | RW | no |
| mc.control.overvoltage_level_6_threshold |  | 42308 | 1 | U16 | 10 | V | RW | no |
| mc.control.overvoltage_level_6_duration |  | 42309 | 2 | U32 | 1 | ms | RW | no |
| mc.control.undervoltage_level_1_threshold |  | 42311 | 1 | U16 | 10 | V | RW | no |
| mc.control.undervoltage_level_1_duration |  | 42312 | 2 | U32 | 1 | ms | RW | no |
| mc.control.undervoltage_level_2_threshold |  | 42314 | 1 | U16 | 10 | V | RW | no |
| mc.control.undervoltage_level_2_duration |  | 42315 | 2 | U32 | 1 | ms | RW | no |
| mc.control.undervoltage_level_3_threshold |  | 42317 | 1 | U16 | 10 | V | RW | no |
| mc.control.undervoltage_level_3_duration |  | 42318 | 2 | U32 | 1 | ms | RW | no |
| mc.control.undervoltage_level_4_threshold |  | 42320 | 1 | U16 | 10 | V | RW | no |
| mc.control.undervoltage_level_4_duration |  | 42321 | 2 | U32 | 1 | ms | RW | no |
| mc.control.undervoltage_level_5_threshold |  | 42323 | 1 | U16 | 10 | V | RW | no |
| mc.control.undervoltage_level_5_duration |  | 42324 | 2 | U32 | 1 | ms | RW | no |
| mc.control.undervoltage_level_6_threshold |  | 42326 | 1 | U16 | 10 | V | RW | no |
| mc.control.undervoltage_level_6_duration |  | 42327 | 2 | U32 | 1 | ms | RW | no |
| mc.control.overfrequency_level_1_threshold |  | 42329 | 1 | U16 | 100 | Hz | RW | no |
| mc.control.overfrequency_level_1_duration |  | 42330 | 2 | U32 | 1 | ms | RW | no |
| mc.control.overfrequency_level_2_threshold |  | 42332 | 1 | U16 | 100 | Hz | RW | no |
| mc.control.overfrequency_level_2_duration |  | 42333 | 2 | U32 | 1 | ms | RW | no |
| mc.control.overfrequency_level_3_threshold |  | 42335 | 1 | U16 | 100 | Hz | RW | no |
| mc.control.overfrequency_level_3_duration |  | 42336 | 2 | U32 | 1 | ms | RW | no |
| mc.control.overfrequency_level_4_threshold |  | 42338 | 1 | U16 | 100 | Hz | RW | no |
| mc.control.overfrequency_level_4_duration |  | 42339 | 2 | U32 | 1 | ms | RW | no |
| mc.control.overfrequency_level_5_threshold |  | 42341 | 1 | U16 | 100 | Hz | RW | no |
| mc.control.overfrequency_level_5_duration |  | 42342 | 2 | U32 | 1 | ms | RW | no |
| mc.control.overfrequency_level_6_threshold |  | 42344 | 1 | U16 | 100 | Hz | RW | no |
| mc.control.overfrequency_level_6_duration |  | 42345 | 2 | U32 | 1 | ms | RW | no |
| mc.control.underfrequency_level_1_threshold |  | 42347 | 1 | U16 | 100 | Hz | RW | no |
| mc.control.underfrequency_level_1_duration |  | 42348 | 2 | U32 | 1 | ms | RW | no |
| mc.control.underfrequency_level_2_threshold |  | 42350 | 1 | U16 | 100 | Hz | RW | no |
| mc.control.underfrequency_level_2_duration |  | 42351 | 2 | U32 | 1 | ms | RW | no |
| mc.control.underfrequency_level_3_threshold |  | 42353 | 1 | U16 | 100 | Hz | RW | no |
| mc.control.underfrequency_level_3_duration |  | 42354 | 2 | U32 | 1 | ms | RW | no |
| mc.control.underfrequency_level_4_threshold |  | 42356 | 1 | U16 | 100 | Hz | RW | no |
| mc.control.underfrequency_level_4_duration |  | 42357 | 2 | U32 | 1 | ms | RW | no |
| mc.control.underfrequency_level_5_threshold |  | 42359 | 1 | U16 | 100 | Hz | RW | no |
| mc.control.underfrequency_level_5_duration |  | 42360 | 2 | U32 | 1 | ms | RW | no |
| mc.control.underfrequency_level_6_threshold |  | 42362 | 1 | U16 | 100 | Hz | RW | no |
| mc.control.underfrequency_level_6_duration |  | 42363 | 2 | U32 | 1 | ms | RW | no |
| mc.control.ramps_42100 |  | 42100 | 1 | U16 | 1 | s | RW | no |
| mc.control.ride_through_42110 |  | 42110 | 1 | E16 | not stated | not stated | RW | yes |
| mc.control.grid_jump_threshold |  | 42111 | 1 | U16 | 10 | % | RW | no |
| mc.control.ride_through_42112 |  | 42112 | 1 | E16 | not stated | not stated | RW | yes |
| mc.control.ride_through_42113 |  | 42113 | 1 | U16 | 10 | V | RW | yes |
| mc.control.ride_through_42114 |  | 42114 | 1 | U16 | 10 | not stated | RW | yes |
| mc.control.ride_through_42118 |  | 42118 | 1 | U16 | 100 | not stated | RW | yes |
| mc.control.ride_through_42119 |  | 42119 | 1 | E16 | not stated | not stated | RW | no |
| mc.control.ride_through_42120 |  | 42120 | 1 | U16 | 10 | V | RW | no |
| mc.control.ride_through_42121 |  | 42121 | 1 | E16 | not stated | not stated | RW | no |
| mc.control.ride_through_42122 |  | 42122 | 1 | U16 | 10 | not stated | RW | no |
| mc.control.vrt_exit_hysteresis |  | 42123 | 1 | U16 | 10 | V | RW | no |
| mc.control.ride_through_42124 |  | 42124 | 1 | U16 | 1 | % | RW | yes |
| mc.control.ride_through_42125 |  | 42125 | 1 | U16 | 1 | %/s | RW | yes |
| mc.control.ride_through_42126 |  | 42126 | 1 | U16 | 10 | not stated | RW | yes |
| mc.control.ride_through_42127 |  | 42127 | 1 | U16 | 10 | not stated | RW | no |
| mc.control.ride_through_42128 |  | 42128 | 1 | E16 | not stated | not stated | RW | yes |
| mc.control.ride_through_42133 |  | 42133 | 1 | U16 | 1 | % | RW | no |
| mc.control.lvrt_zero_current_threshold_conflict |  | 42134 | 1 | U16 | 10 | V | RW | no |
| mc.control.ride_through_42135 |  | 42135 | 1 | E16 | not stated | not stated | RW | yes |
| mc.control.voltage_rise_42138 |  | 42138 | 1 | E16 | not stated | not stated | RW | yes |
| mc.control.voltage_rise_reactive_point |  | 42139 | 1 | U16 | 10 | % | RW | no |
| mc.control.voltage_rise_active_point |  | 42140 | 1 | U16 | 10 | % | RW | no |
| mc.control.islanding_42129 |  | 42129 | 1 | E16 | not stated | not stated | RW | yes |
| mc.control.islanding_42130 |  | 42130 | 1 | E16 | not stated | not stated | RW | yes |
| mc.control.islanding_42131 |  | 42131 | 1 | E16 | not stated | not stated | RW | yes |
| mc.control.normal_connection_42084 |  | 42084 | 1 | U16 | 10 | V | RW | no |
| mc.control.normal_connection_42085 |  | 42085 | 1 | U16 | 10 | V | RW | no |
| mc.control.normal_connection_42086 |  | 42086 | 1 | U16 | 100 | Hz | RW | no |
| mc.control.normal_connection_42087 |  | 42087 | 1 | U16 | 100 | Hz | RW | no |
| mc.control.startup_recovery_42088 |  | 42088 | 1 | U16 | 10 | V | RW | no |
| mc.control.startup_recovery_42089 |  | 42089 | 1 | U16 | 10 | V | RW | no |
| mc.control.startup_recovery_42090 |  | 42090 | 1 | U16 | 100 | Hz | RW | no |
| mc.control.startup_recovery_42091 |  | 42091 | 1 | U16 | 100 | Hz | RW | no |
| mc.control.startup_recovery_42105 |  | 42105 | 2 | U32 | 1 | ms | RW | no |
| mc.control.rapid_restart_mode |  | 42116 | 1 | E16 | not stated | not stated | RW | no |

## COMMERCIAL

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| commercial.control.dispatch.40120 |  | 40120 | 1 | U16 | 10 | kW | RW | yes |
| commercial.control.dispatch.40122 |  | 40122 | 1 | I16 | 1000 | not stated | RW | yes |
| commercial.control.dispatch.40123 |  | 40123 | 1 | I16 | 1000 | not stated | RW | yes |
| commercial.control.dispatch.40125 |  | 40125 | 1 | U16 | 10 | % | RW | yes |
| commercial.control.dispatch.40200 |  | 40200 | 1 | U16 | 1 | not stated | RW | yes |
| commercial.control.dispatch.40201 |  | 40201 | 1 | U16 | 1 | not stated | RW | yes |

## HA

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| ha.control.dispatch.40037 |  | 40037 | 1 | U16 | 1 | not stated | RW | yes |
| ha.control.dispatch.40038 |  | 40038 | 1 | U16 | 1 | % | RW | yes |
| ha.control.dispatch.40120 |  | 40120 | 1 | U16 | 10 | kW | RW | yes |
| ha.control.dispatch.40122 |  | 40122 | 1 | I16 | 1000 | not stated | RW | yes |
| ha.control.dispatch.40123 |  | 40123 | 1 | I16 | 1000 | not stated | RW | yes |
| ha.control.dispatch.40124 |  | 40124 | 1 | U16 | 10 | s | RW | yes |
| ha.control.dispatch.40125 |  | 40125 | 1 | U16 | 10 | % | RW | yes |
| ha.control.dispatch.40129 |  | 40129 | 2 | I32 | 1000 | kVar | RW | yes |
| ha.control.dispatch.40133 |  | 40133 | 21 | MLD | 1 | not stated | RW | no |
| ha.control.dispatch.40154 |  | 40154 | 21 | MLD | 1 | not stated | RW | no |
| ha.control.dispatch.40175 |  | 40175 | 21 | MLD | 1 | not stated | RW | no |
| ha.control.dispatch.40196 |  | 40196 | 1 | U16 | 1 | s | RW | yes |
| ha.control.dispatch.40198 |  | 40198 | 1 | U16 | 1 | % | RW | yes |
| ha.control.dispatch.40200 |  | 40200 | 1 | U16 | 1 | not stated | WO | yes |
| ha.control.dispatch.40201 |  | 40201 | 1 | U16 | 1 | not stated | WO | yes |
| ha.control.dispatch.40205 |  | 40205 | 1 | U16 | 1 | not stated | WO | yes |
| ha.control.grid_protection.42000 |  | 42000 | 1 | U16 | 1 | not stated | RW | yes |
| ha.control.grid_protection.42015 |  | 42015 | 2 | U32 | 1000 | %/s | RW | yes |
| ha.control.grid_protection.42017 |  | 42017 | 2 | U32 | 1000 | %/s | RW | yes |
| ha.control.grid_protection.42019 |  | 42019 | 2 | U32 | 1 | s | RW | yes |
| ha.control.grid_protection.42067 |  | 42067 | 1 | ENUM16 | not stated | not stated | RW | yes |
| dispatch.snapshot.adjustmentmode |  | 35300 | 1 | U16 | 1 | - | RO | no |
| dispatch.snapshot.adjustmentmode |  | 35304 | 1 | U16 | 1 | - | RO | no |
| dispatch.snapshot.adjustmentvalue |  | 35305 | 2 | U32 | 1 | - | RO | no |
| dispatch.snapshot.adjustmentcommand |  | 35307 | 1 | U16 | 1 | - | RO | no |
| time.event_or_collection.timeofthelasthighvoltsride_throughs |  | 35318 | 2 | U32 | 1 | - | RO | no |
| time.event_or_collection.timeofthelastlowvoltsride_throughs |  | 35320 | 2 | U32 | 1 | - | RO | no |
| inverter.measurement.cumulativenumberofhighvoltsride_throughs |  | 35322 | 2 | U32 | 1 | - | RO | no |
| inverter.measurement.cumulativenumberoflowvoltsride_throughs |  | 35324 | 2 | U32 | 1 | - | RO | no |
| time.event_or_collection.exittimeofthelasthighvoltsride_throughs |  | 35493 | 2 | U32 | 1 | - | RO | no |
| time.event_or_collection.exittimeofthelastlowvoltsride_throughs |  | 35495 | 2 | U32 | 1 | - | RO | no |

## HB

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| hb.control.dispatch.40037 |  | 40037 | 1 | U16 | 1 | not stated | RW | yes |
| hb.control.dispatch.40038 |  | 40038 | 1 | U16 | 1 | % | RW | yes |
| hb.control.dispatch.40039 |  | 40039 | 1 | I16 | 100 | % | RW | yes |
| hb.control.dispatch.40040 |  | 40040 | 1 | I16 | 100 | % | RW | yes |
| hb.control.dispatch.40043 |  | 40043 | 2 | I32 | 1000 | kW | RW | yes |
| hb.control.dispatch.40122 |  | 40122 | 1 | I16 | 1000 | not stated | RW | yes |
| hb.control.dispatch.40129 |  | 40129 | 1 | I32 | 1000 | kVar | RW | yes |
| hb.control.dispatch.40133 |  | 40133 | 21 | MLD | 1 | not stated | RW | no |
| hb.control.dispatch.40154 |  | 40154 | 21 | MLD | 1 | not stated | RW | no |
| hb.control.dispatch.40175 |  | 40175 | 21 | MLD | 1 | not stated | RW | no |
| hb.control.dispatch.40196 |  | 40196 | 1 | U16 | 1 | s | RW | yes |
| hb.control.dispatch.40198 |  | 40198 | 1 | U16 | 1 | % | RW | yes |
| hb.control.dispatch.40200 |  | 40200 | 1 | U16 | 1 | not stated | WO | yes |
| hb.control.dispatch.40201 |  | 40201 | 1 | U16 | 1 | not stated | WO | yes |
| hb.control.dispatch.40354 |  | 40354 | 21 | MLD | 1 | not stated | RW | no |
| hb.control.grid_protection.42000 |  | 42000 | 1 | U16 | 1 | not stated | RW | yes |
| hb.control.grid_protection.42015 |  | 42015 | 2 | U32 | 1000 | %/s | RW | yes |
| hb.control.grid_protection.42017 |  | 42017 | 2 | U32 | 1000 | %/s | RW | yes |
| hb.control.grid_protection.42019 |  | 42019 | 2 | U32 | 1 | s | RW | yes |
| dispatch.snapshot.regulationmode |  | 35300 | 1 | U16 | 1 | - | RO | no |
| dispatch.snapshot.adjustmentvalue |  | 35301 | 2 | U32 | * | - | RO | no |
| dispatch.snapshot.adjustmentinstruction |  | 35303 | 1 | U16 | 1 | - | RO | no |
| dispatch.snapshot.regulationmode |  | 35304 | 1 | U16 | 1 | - | RO | no |
| dispatch.snapshot.adjustmentvalue |  | 35305 | 2 | U32 | * | - | RO | no |
| dispatch.snapshot.adjustmentinstruction |  | 35307 | 1 | U16 | 1 | - | RO | no |

## HD

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| hd.control.dispatch.40037 |  | 40037 | 1 | U16 | 1 | not stated | RW | yes |
| hd.control.dispatch.40038 |  | 40038 | 1 | U16 | 1 | % | RW | yes |
| hd.control.dispatch.40120 |  | 40120 | 1 | U16 | 10 | kW | RW | yes |
| hd.control.dispatch.40122 |  | 40122 | 1 | I16 | 1000 | not stated | RW | yes |
| hd.control.dispatch.40123 |  | 40123 | 1 | I16 | 1000 | not stated | RW | yes |
| hd.control.dispatch.40124 |  | 40124 | 1 | U16 | 10 | s | RW | yes |
| hd.control.dispatch.40125 |  | 40125 | 1 | U16 | 10 | % | RW | yes |
| hd.control.dispatch.40129 |  | 40129 | 2 | I32 | 1000 | kVar | RW | yes |
| hd.control.dispatch.40133 |  | 40133 | 21 | MLD | 1 | not stated | RW | no |
| hd.control.dispatch.40154 |  | 40154 | 21 | MLD | 1 | not stated | RW | no |
| hd.control.dispatch.40175 |  | 40175 | 21 | MLD | 1 | not stated | RW | no |
| hd.control.dispatch.40196 |  | 40196 | 1 | U16 | 1 | s | RW | yes |
| hd.control.dispatch.40198 |  | 40198 | 1 | U16 | 1 | % | RW | yes |
| hd.control.dispatch.40200 |  | 40200 | 1 | U16 | 1 | not stated | WO | yes |
| hd.control.dispatch.40201 |  | 40201 | 1 | U16 | 1 | not stated | WO | yes |
| hd.control.grid_protection.42000 |  | 42000 | 1 | U16 | 1 | not stated | RW | yes |
| hd.control.grid_protection.42014 |  | 42014 | 1 | ENUM16 | 1 | not stated | RW | yes |
| hd.control.grid_protection.42015 |  | 42015 | 2 | U32 | 1000 | %/s | RW | yes |
| hd.control.grid_protection.42017 |  | 42017 | 2 | U32 | 1000 | %/s | RW | yes |
| hd.control.grid_protection.42019 |  | 42019 | 2 | U32 | 1 | s | RW | yes |
| hd.control.grid_protection.42067 |  | 42067 | 1 | ENUM16 | 1 | not stated | RW | yes |
| hd.control.grid_protection.42589 |  | 42589 | 1 | ENUM16 | 1 | not stated | RW | yes |
| hd.control.grid_protection.42606 |  | 42606 | 1 | ENUM16 | 1 | not stated | RW | yes |
| hd.control.grid_protection.42607 |  | 42607 | 1 | ENUM16 | 1 | not stated | RW | yes |
| hd.control.grid_protection.42608 |  | 42608 | 1 | ENUM16 | 1 | not stated | RW | yes |
| hd.control.grid_protection.42609 |  | 42609 | 1 | U16 | 1 | min | RW | yes |
| dispatch.snapshot.adjustmentmode |  | 35300 | 4 | U16 | 1 | - | RO | no |
| dispatch.snapshot.adjustmentmode |  | 35304 | 4 | U16 | 1 | - | RO | no |
| grid.ride_through_count.lvrttimes |  | 35483 | 2 | U32 | 1 | - | RO | no |
| grid.ride_through_count.hvrttimes |  | 35485 | 2 | U32 | 1 | - | RO | no |

## SUN-C81

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| sun-c81.control.dispatch.40117 | 1 | 40117 | 1 | U16 | 1 | not stated | RW | yes |
| sun-c81.control.dispatch.40117 | 2 | 40117 | 1 | U16 | 1 | not stated | RW | yes |
| sun-c81.control.dispatch.40118 | 1 | 40118 | 1 | U16 | 1 | not stated | RW | yes |
| sun-c81.control.dispatch.40118 | 2 | 40118 | 1 | U16 | 1 | not stated | RW | yes |
| sun-c81.control.dispatch.40119 | 1 | 40119 | 1 | U16 | 1 | % | RW | yes |
| sun-c81.control.dispatch.40119 | 2 | 40119 | 1 | U16 | 1 | % | RW | yes |
| sun-c81.control.dispatch.40120 | 1 | 40120 | 1 | U16 | 10 | kW | RW | yes |
| sun-c81.control.dispatch.40120 | 2 | 40120 | 1 | U16 | 10 | kW | RW | yes |
| sun-c81.control.dispatch.40121 | 1 | 40121 | 1 | U16 | 10 | %/s | RW | yes |
| sun-c81.control.dispatch.40121 | 2 | 40121 | 1 | U16 | 10 | %/s | RW | yes |
| sun-c81.control.dispatch.40122 | 1 | 40122 | 1 | I16 | 1000 | not stated | RW | yes |
| sun-c81.control.dispatch.40122 | 2 | 40122 | 1 | I16 | 1000 | not stated | RW | yes |
| sun-c81.control.dispatch.40123 | 1 | 40123 | 1 | I16 | 1000 | not stated | RW | yes |
| sun-c81.control.dispatch.40123 | 2 | 40123 | 1 | I16 | 1000 | not stated | RW | yes |
| sun-c81.control.dispatch.40124 | 1 | 40124 | 1 | U16 | 1 | s | RW | yes |
| sun-c81.control.dispatch.40124 | 2 | 40124 | 1 | U16 | 1 | s | RW | yes |
| sun-c81.control.dispatch.40125 | 1 | 40125 | 1 | U16 | 10 | % | RW | yes |
| sun-c81.control.dispatch.40125 | 2 | 40125 | 1 | U16 | 10 | % | RW | yes |
| sun-c81.control.dispatch.40133 |  | 40133 | 21 | BYTES | 1 | not stated | RW | yes |
| sun-c81.control.dispatch.40154 |  | 40154 | 21 | BYTES | 1 | not stated | RW | yes |
| sun-c81.control.dispatch.40198 |  | 40198 | 1 | U16 | 1 | % | RW | yes |
| sun-c81.control.dispatch.40200 |  | 40200 | 1 | U16 | 1 | not stated | WO | yes |
| sun-c81.control.dispatch.40201 |  | 40201 | 1 | U16 | 1 | not stated | WO | yes |
| sun-c81.control.dispatch.40232 | 1 | 40232 | 1 | U16 | 10 | % | WO | yes |
| sun-c81.control.dispatch.40232 | 2 | 40232 | 1 | U16 | 10 | % | WO | yes |
| sun-c81.control.dispatch.40234 | 1 | 40234 | 1 | U16 | 1 | % | WO | yes |
| sun-c81.control.dispatch.40234 | 2 | 40234 | 1 | U16 | 1 | % | WO | yes |
| sun-c81.control.dispatch.40235 | 1 | 40235 | 1 | U16 | 10 | kW | WO | yes |
| sun-c81.control.dispatch.40235 | 2 | 40235 | 1 | U16 | 10 | kW | WO | yes |
| sun-c81.control.dispatch.40236 | 1 | 40236 | 1 | I16 | 1000 | not stated | WO | yes |
| sun-c81.control.dispatch.40236 | 2 | 40236 | 1 | I16 | 1000 | not stated | WO | yes |
| sun-c81.control.dispatch.40237 | 1 | 40237 | 1 | I16 | 1000 | not stated | WO | yes |
| sun-c81.control.dispatch.40237 | 2 | 40237 | 1 | I16 | 1000 | not stated | WO | yes |
| sun-c81.control.grid_protection.42045 |  | 42045 | 2 | U32 | 1 | ms | RW | yes |
| sun-c81.control.grid_protection.42047 |  | 42047 | 2 | U32 | 1 | ms | RW | yes |
| sun-c81.control.grid_protection.42049 |  | 42049 | 2 | U32 | 1 | ms | RW | yes |
| sun-c81.control.grid_protection.42051 |  | 42051 | 2 | U32 | 1 | ms | RW | yes |
| sun-c81.control.grid_protection.42053 |  | 42053 | 2 | U32 | 1 | ms | RW | yes |
| sun-c81.control.grid_protection.42055 |  | 42055 | 2 | U32 | 1 | ms | RW | yes |
| sun-c81.control.grid_protection.42057 |  | 42057 | 2 | U32 | 1 | ms | RW | yes |
| sun-c81.control.grid_protection.42059 |  | 42059 | 2 | U32 | 1 | ms | RW | yes |
| sun-c81.control.grid_protection.42061 |  | 42061 | 2 | U32 | 1 | ms | RW | yes |
| sun-c81.control.grid_protection.42063 |  | 42063 | 1 | U16 | 10 | V | RW | yes |
| sun-c81.control.grid_protection.42064 |  | 42064 | 1 | U16 | 10 | V | RW | yes |
| sun-c81.control.grid_protection.42065 |  | 42065 | 1 | U16 | 10 | V | RW | yes |
| sun-c81.control.grid_protection.42066 |  | 42066 | 1 | U16 | 10 | V | RW | yes |
| sun-c81.control.grid_protection.42067 |  | 42067 | 1 | U16 | 100 | Hz | RW | yes |
| sun-c81.control.grid_protection.42068 |  | 42068 | 1 | U16 | 100 | Hz | RW | yes |
| sun-c81.control.grid_protection.42069 |  | 42069 | 1 | U16 | 100 | Hz | RW | yes |
| sun-c81.control.grid_protection.42070 |  | 42070 | 1 | U16 | 100 | Hz | RW | yes |
| sun-c81.control.grid_protection.42071 |  | 42071 | 1 | U16 | 10 | V | RW | yes |
| sun-c81.control.grid_protection.42072 |  | 42072 | 1 | U16 | 1 | not stated | RW | yes |
| sun-c81.control.grid_protection.42074 |  | 42074 | 1 | U16 | 1000 | MΩ | RW | yes |
| sun-c81.control.grid_protection.42075 |  | 42075 | 1 | U16 | 10 | % | RW | yes |
| sun-c81.control.grid_protection.42083 |  | 42083 | 1 | U16 | 1 | s | RW | yes |
| sun-c81.control.grid_protection.42084 |  | 42084 | 1 | U16 | 1 | not stated | RW | yes |
| sun-c81.control.grid_protection.42085 |  | 42085 | 1 | U16 | 1 | s | RW | yes |
| sun-c81.control.grid_protection.42086 |  | 42086 | 1 | U16 | 1 | s | RW | yes |
| sun-c81.control.grid_protection.42087 |  | 42087 | 1 | U16 | 1 | not stated | RW | yes |
| sun-c81.control.grid_protection.42089 |  | 42089 | 1 | U16 | 10 | not stated | RW | yes |
| sun-c81.control.grid_protection.42090 |  | 42090 | 1 | U16 | 1 | % | RW | yes |
| sun-c81.control.grid_protection.42091 |  | 42091 | 1 | U16 | 1 | % | RW | yes |
| sun-c81.control.grid_protection.42092 |  | 42092 | 1 | U16 | 100 | Hz | RW | yes |
| sun-c81.control.grid_protection.42093 |  | 42093 | 1 | U16 | 100 | Hz | RW | yes |
| sun-c81.control.grid_protection.42094 |  | 42094 | 1 | U16 | 1 | %/min | RW | yes |
| sun-c81.control.grid_protection.42095 |  | 42095 | 1 | U16 | 1 | not stated | RW | yes |
| sun-c81.control.grid_protection.42096 |  | 42096 | 1 | U16 | 1 | % | RW | yes |
| sun-c81.control.grid_protection.42097 |  | 42097 | 1 | U16 | 1 | not stated | RW | yes |
| sun-c81.control.grid_protection.42101 |  | 42101 | 1 | U16 | 1 | min | RW | yes |
| sun-c81.control.grid_protection.42125 |  | 42125 | 1 | U16 | 1 | not stated | RW | yes |
| sun-c81.control.grid_protection.42151 |  | 42151 | 1 | U16 | 10 | V | RW | yes |
| sun-c81.control.grid_protection.42152 |  | 42152 | 2 | U32 | 1 | ms | RW | yes |
| sun-c81.control.grid_protection.42154 |  | 42154 | 1 | U16 | 10 | V | RW | yes |
| sun-c81.control.grid_protection.42155 |  | 42155 | 2 | U32 | 1 | ms | RW | yes |
| sun-c81.control.grid_protection.42157 |  | 42157 | 1 | U16 | 10 | V | RW | yes |
| sun-c81.control.grid_protection.42158 |  | 42158 | 2 | U32 | 1 | ms | RW | yes |
| sun-c81.control.grid_protection.42160 |  | 42160 | 1 | U16 | 10 | V | RW | yes |
| sun-c81.control.grid_protection.42161 |  | 42161 | 2 | U32 | 1 | ms | RW | yes |
| sun-c81.control.grid_protection.42174 |  | 42174 | 1 | U16 | 1 | not stated | RW | yes |
| sun-c81.control.grid_protection.42320 |  | 42320 | 1 | U16 | 1 | % | RW | yes |
| sun-c81.control.grid_protection.42321 |  | 42321 | 1 | I16 | 1000 | not stated | RW | yes |
| sun-c81.control.grid_protection.42333 |  | 42333 | 1 | U16 | 1 | not stated | RW | yes |
| sun-c81.control.grid_protection.42731 |  | 42731 | 1 | U16 | 1 | not stated | WO | yes |
| dispatch.snapshot.adjustmentmode |  | 32402 | 1 | U16 | 1 | N/A | RO | no |
| dispatch.snapshot.adjustmentvalue |  | 32403 | 2 | U32 | * | N/A | RO | no |
| dispatch.snapshot.adjustmentcommand |  | 32405 | 1 | U16 | 1 | N/A | RO | no |
| dispatch.snapshot.adjustmentmode |  | 32406 | 1 | U16 | 1 | N/A | RO | no |
| dispatch.snapshot.adjustmentvalue |  | 32407 | 2 | U32 | * | N/A | RO | no |
| dispatch.snapshot.adjustmentcommand |  | 32409 | 1 | U16 | 1 | N/A | RO | no |

## SUN-V300

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| sun-v300.control.dispatch.40037 |  | 40037 | 1 | U16 | 1 | not stated | RW | yes |
| sun-v300.control.dispatch.40038 |  | 40038 | 1 | U16 | 1 | % | RW | yes |
| sun-v300.control.dispatch.40120 |  | 40120 | 1 | U16 | 10 | kW | RW | yes |
| sun-v300.control.dispatch.40122 |  | 40122 | 1 | I16 | 1000 | not stated | RW | yes |
| sun-v300.control.dispatch.40123 |  | 40123 | 1 | I16 | 1000 | not stated | RW | yes |
| sun-v300.control.dispatch.40125 |  | 40125 | 1 | U16 | 10 | % | RW | yes |
| sun-v300.control.dispatch.40129 |  | 40129 | 2 | I32 | 1000 | kVar | RW | yes |
| sun-v300.control.dispatch.40133 |  | 40133 | 21 | MLD | 1 | not stated | RW | no |
| sun-v300.control.dispatch.40154 |  | 40154 | 21 | MLD | 1 | not stated | RW | no |
| sun-v300.control.dispatch.40175 |  | 40175 | 21 | MLD | 1 | not stated | RW | no |
| sun-v300.control.dispatch.40196 |  | 40196 | 1 | U16 | 1 | s | RW | yes |
| sun-v300.control.dispatch.40198 |  | 40198 | 1 | U16 | 1 | % | RW | yes |
| sun-v300.control.dispatch.40200 |  | 40200 | 1 | U16 | 1 | not stated | WO | yes |
| sun-v300.control.dispatch.40201 |  | 40201 | 1 | U16 | 1 | not stated | WO | yes |
| sun-v300.control.grid_protection.42000 |  | 42000 | 1 | U16 | 1 | not stated | RW | yes |
| sun-v300.control.grid_protection.42015 |  | 42015 | 2 | U32 | 1000 | %/s | RW | yes |
| sun-v300.control.grid_protection.42017 |  | 42017 | 2 | U32 | 1000 | %/s | RW | yes |
| sun-v300.control.grid_protection.42019 |  | 42019 | 2 | U32 | 1 | s | RW | yes |
| dispatch.snapshot.adjustmentmode |  | 35300 | 1 | U16 | 1 | N/A | RO | no |
| dispatch.snapshot.adjustmentvalue |  | 35302 | 2 | U32 | * | N/A | RO | no |
| dispatch.snapshot.adjustmentcommand |  | 35303 | 1 | U16 | 1 | N/A | RO | no |
| dispatch.snapshot.adjustmentmode |  | 35304 | 1 | U16 | 1 | N/A | RO | no |
| dispatch.snapshot.adjustmentvalue |  | 35305 | 2 | U32 | * | N/A | RO | no |
| dispatch.snapshot.adjustmentcommand |  | 35307 | 1 | U16 | 1 | N/A | RO | no |

## emma-v100r024c00-01-2024-07-15

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway-self.40100 |  | 40100 | 1 | ENUM16 | NA | NA | RW | no |

## emma-v100r025c00spc102-2025-08-14

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway-self.40100 |  | 40100 | 1 | ENUM16 | NA | NA | RW | no |

## s-dongle-v100r001c00-05-2022-07-06

s-dongle-v100r001c00-05-2022-07-06 defines no grid-code row

## s-dongle-v200r022cc10-02-2023-04-20

s-dongle-v200r022cc10-02-2023-04-20 defines no grid-code row

## s-dongle-v200r025c00-01-2025-04-27

s-dongle-v200r025c00-01-2025-04-27 defines no grid-code row

## s-dongle-v200r025c00spc120-01-2025-12-23

s-dongle-v200r025c00spc120-01-2025-12-23 defines no grid-code row

## smartlogger-32-2018-11-15

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway-self.40737 |  | 40737 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40738 |  | 40738 | 2 | U32 | 10 | kW | RO | no |
| gateway-self.40740 |  | 40740 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40741 |  | 40741 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40742 |  | 40742 | 2 | I32 | 10/1000 | kVar | RO | no |
| gateway-self.40420 |  | 40420 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40422 |  | 40422 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40424 |  | 40424 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40426 |  | 40426 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40428 |  | 40428 | 1 | U16 | 10 | % | RW | no |
| gateway-self.40429 |  | 40429 | 1 | I16 | 1000 | N/A | RW | no |

## smartlogger-36-2020-04-13

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway-self.40737 |  | 40737 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40738 |  | 40738 | 2 | U32 | 10 | kW | RO | no |
| gateway-self.40740 |  | 40740 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40741 |  | 40741 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40742 |  | 40742 | 2 | I32 | 10/1000 | kVar | RO | no |
| gateway-self.40420 |  | 40420 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40422 |  | 40422 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40424 |  | 40424 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40426 |  | 40426 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40428 |  | 40428 | 1 | U16 | 10 | % | RW | no |
| gateway-self.40429 |  | 40429 | 1 | I16 | 1000 | N/A | RW | no |

## smartlogger-37-2020-06-24

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway-self.40737 |  | 40737 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40738 |  | 40738 | 2 | U32 | 10 | kW | RO | no |
| gateway-self.40740 |  | 40740 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40741 |  | 40741 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40742 |  | 40742 | 2 | I32 | 10/1000 | kVar | RO | no |
| gateway-self.40420 |  | 40420 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40422 |  | 40422 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40424 |  | 40424 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40426 |  | 40426 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40428 |  | 40428 | 1 | U16 | 10 | % | RW | no |
| gateway-self.40429 |  | 40429 | 1 | I16 | 1000 | N/A | RW | no |

## smartlogger-v300r024c10-01-2026-10-15

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway-self.40737 |  | 40737 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40738 |  | 40738 | 2 | I32 | 10 | kW | RO | no |
| gateway-self.40740 |  | 40740 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40741 |  | 40741 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40742 |  | 40742 | 2 | I32 | 10/1000 | kVar | RO | no |
| gateway-self.40378 |  | 40378 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40381 |  | 40381 | 2 | I32 | 10 | kW | RW | no |
| gateway-self.40384 | 1 | 40384 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40384 | 2 | 40384 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40386 |  | 40386 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40420 |  | 40420 | 2 | I32 | 10 | kW | RW | no |
| gateway-self.40422 |  | 40422 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40424 |  | 40424 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40426 |  | 40426 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40428 |  | 40428 | 1 | I16 | 10 | % | RW | no |
| gateway-self.40429 |  | 40429 | 1 | I16 | 1000 | N/A | RW | no |
| gateway-self.40430 |  | 40430 | 2 | I32 | 1000 | kW | RW | no |
| gateway-self.40432 |  | 40432 | 2 | I32 | 1000 | kVar | RW | no |
| gateway-self.44165 |  | 44165 | 1 | U16 | N/A | N/A | RW | no |

## smartlogger-v300r024c10-53-2025-10-29

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway-self.40737 |  | 40737 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40738 |  | 40738 | 2 | I32 | 10 | kW | RO | no |
| gateway-self.40740 |  | 40740 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40741 |  | 40741 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40742 |  | 40742 | 2 | I32 | 10/1000 | kVar | RO | no |
| gateway-self.40378 |  | 40378 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40381 |  | 40381 | 2 | I32 | 10 | kW | RW | no |
| gateway-self.40384 | 1 | 40384 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40384 | 2 | 40384 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40386 |  | 40386 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40420 |  | 40420 | 2 | I32 | 10 | kW | RW | no |
| gateway-self.40422 |  | 40422 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40424 |  | 40424 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40426 |  | 40426 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40428 |  | 40428 | 1 | I16 | 10 | % | RW | no |
| gateway-self.40429 |  | 40429 | 1 | I16 | 1000 | N/A | RW | no |
| gateway-self.40430 |  | 40430 | 2 | I32 | 1000 | kW | RW | no |
| gateway-self.40432 |  | 40432 | 2 | I32 | 1000 | kVar | RW | no |

## smartlogger-v300r024c10-54-2026-02-06

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway-self.40737 |  | 40737 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40738 |  | 40738 | 2 | I32 | 10 | kW | RO | no |
| gateway-self.40740 |  | 40740 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40741 |  | 40741 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40742 |  | 40742 | 2 | I32 | 10/1000 | kVar | RO | no |
| gateway-self.40378 |  | 40378 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40381 |  | 40381 | 2 | I32 | 10 | kW | RW | no |
| gateway-self.40384 | 1 | 40384 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40384 | 2 | 40384 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40386 |  | 40386 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40420 |  | 40420 | 2 | I32 | 10 | kW | RW | no |
| gateway-self.40422 |  | 40422 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40424 |  | 40424 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40426 |  | 40426 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40428 |  | 40428 | 1 | I16 | 10 | % | RW | no |
| gateway-self.40429 |  | 40429 | 1 | I16 | 1000 | N/A | RW | no |
| gateway-self.40430 |  | 40430 | 2 | I32 | 1000 | kW | RW | no |
| gateway-self.40432 |  | 40432 | 2 | I32 | 1000 | kVar | RW | no |

## smartlogger-v300r024c10spc180-01-2025-06-23

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway-self.40737 |  | 40737 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40738 |  | 40738 | 2 | I32 | 10 | kW | RO | no |
| gateway-self.40740 |  | 40740 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40741 |  | 40741 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40742 |  | 40742 | 2 | I32 | 10/1000 | kVar | RO | no |
| gateway-self.40378 |  | 40378 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40381 |  | 40381 | 2 | I32 | 10 | kW | RW | no |
| gateway-self.40384 | 1 | 40384 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40384 | 2 | 40384 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40386 |  | 40386 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40420 |  | 40420 | 2 | I32 | 10 | kW | RW | no |
| gateway-self.40422 |  | 40422 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40424 |  | 40424 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40426 |  | 40426 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40428 |  | 40428 | 1 | I16 | 10 | % | RW | no |
| gateway-self.40429 |  | 40429 | 1 | I16 | 1000 | N/A | RW | no |
| gateway-self.40430 |  | 40430 | 2 | I32 | 1000 | kW | RW | no |
| gateway-self.40432 |  | 40432 | 2 | I32 | 1000 | kVar | RW | no |

## smartlogger-v300r024c10spc191-52-2025-06-06

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway-self.40737 |  | 40737 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40738 |  | 40738 | 2 | I32 | 10 | kW | RO | no |
| gateway-self.40740 |  | 40740 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40741 |  | 40741 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40742 |  | 40742 | 2 | I32 | 10/1000 | kVar | RO | no |
| gateway-self.40378 |  | 40378 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40381 |  | 40381 | 2 | I32 | 10 | kW | RW | no |
| gateway-self.40384 | 1 | 40384 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40384 | 2 | 40384 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40386 |  | 40386 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40420 |  | 40420 | 2 | I32 | 10 | kW | RW | no |
| gateway-self.40422 |  | 40422 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40424 |  | 40424 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40426 |  | 40426 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40428 |  | 40428 | 1 | I16 | 10 | % | RW | no |
| gateway-self.40429 |  | 40429 | 1 | I16 | 1000 | N/A | RW | no |
| gateway-self.40430 |  | 40430 | 2 | I32 | 1000 | kW | RW | no |
| gateway-self.40432 |  | 40432 | 2 | I32 | 1000 | kVar | RW | no |

## smartlogger-v300r024c10spc210-49-2025-03-19

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway-self.40737 |  | 40737 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40738 |  | 40738 | 2 | I32 | 10 | kW | RO | no |
| gateway-self.40740 |  | 40740 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40741 |  | 40741 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40742 |  | 40742 | 2 | I32 | 10/1000 | kVar | RO | no |
| gateway-self.40378 |  | 40378 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40381 |  | 40381 | 2 | I32 | 10 | kW | RW | no |
| gateway-self.40384 |  | 40384 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40386 |  | 40386 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40420 |  | 40420 | 2 | I32 | 10 | kW | RW | no |
| gateway-self.40422 |  | 40422 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40424 |  | 40424 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40426 |  | 40426 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40428 |  | 40428 | 1 | I16 | 10 | % | RW | no |
| gateway-self.40429 |  | 40429 | 1 | I16 | 1000 | N/A | RW | no |
| gateway-self.40430 |  | 40430 | 2 | I32 | 1000 | kW | RW | no |
| gateway-self.40432 |  | 40432 | 2 | I32 | 1000 | kVar | RW | no |
| gateway-self.41889 |  | 41889 | 1 | U16 | N/A | N/A | RW | no |
| gateway-self.44165 |  | 44165 | 1 | U16 | 1 | N/A | RW | no |

## smartlogger-v300r025c10-01-2025-12-22

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway-self.40737 |  | 40737 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40738 |  | 40738 | 2 | I32 | 10 | kW | RO | no |
| gateway-self.40740 |  | 40740 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40741 |  | 40741 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40742 |  | 40742 | 2 | I32 | 10/1000 | kVar | RO | no |
| gateway-self.40378 |  | 40378 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40381 |  | 40381 | 2 | I32 | 10 | kW | RW | no |
| gateway-self.40384 | 1 | 40384 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40384 | 2 | 40384 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40386 |  | 40386 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40420 |  | 40420 | 2 | I32 | 10 | kW | RW | no |
| gateway-self.40422 |  | 40422 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40424 |  | 40424 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40426 |  | 40426 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40428 |  | 40428 | 1 | I16 | 10 | % | RW | no |
| gateway-self.40429 |  | 40429 | 1 | I16 | 1000 | N/A | RW | no |
| gateway-self.40430 |  | 40430 | 2 | I32 | 1000 | kW | RW | no |
| gateway-self.40432 |  | 40432 | 2 | I32 | 1000 | kVar | RW | no |

## smartlogger-v300r025c10-02-2026-10-09

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway-self.40737 |  | 40737 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40738 |  | 40738 | 2 | I32 | 10 | kW | RO | no |
| gateway-self.40740 |  | 40740 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40741 |  | 40741 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40742 |  | 40742 | 2 | I32 | 10/1000 | kVar | RO | no |
| gateway-self.40378 |  | 40378 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40381 |  | 40381 | 2 | I32 | 10 | kW | RW | no |
| gateway-self.40384 |  | 40384 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40386 |  | 40386 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40420 |  | 40420 | 2 | I32 | 10 | kW | RW | no |
| gateway-self.40422 |  | 40422 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40424 |  | 40424 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40426 |  | 40426 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40429 |  | 40429 | 1 | I16 | 1000 | N/A | RW | no |
| gateway-self.40430 |  | 40430 | 2 | I32 | 1000 | kW | RW | no |
| gateway-self.40432 |  | 40432 | 2 | I32 | 1000 | kVar | RW | no |
| gateway-self.44165 |  | 44165 | 1 | U16 | N/A | N/A | RW | no |

## smartlogger-v300r026c10-02-2026-10-09

| key | occurrence | address | quantity | type | gain | unit | access | addressed by register |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gateway-self.40737 |  | 40737 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40738 |  | 40738 | 2 | I32 | 10 | kW | RO | no |
| gateway-self.40740 |  | 40740 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40741 |  | 40741 | 1 | U16 | 1 | N/A | RO | no |
| gateway-self.40742 |  | 40742 | 2 | I32 | 10/1000 | kVar | RO | no |
| gateway-self.40378 |  | 40378 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40381 |  | 40381 | 2 | I32 | 10 | kW | RW | no |
| gateway-self.40384 |  | 40384 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40386 |  | 40386 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40420 |  | 40420 | 2 | I32 | 10 | kW | RW | no |
| gateway-self.40422 |  | 40422 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40424 |  | 40424 | 2 | U32 | 10 | kW | RW | no |
| gateway-self.40426 |  | 40426 | 2 | I32 | 10 | kVar | RW | no |
| gateway-self.40429 |  | 40429 | 1 | I16 | 1000 | N/A | RW | no |
| gateway-self.40430 |  | 40430 | 2 | I32 | 1000 | kW | RW | no |
| gateway-self.40432 |  | 40432 | 2 | I32 | 1000 | kVar | RW | no |
| gateway-self.44165 |  | 44165 | 1 | U16 | N/A | N/A | RW | no |
