// Package storage is the storage producer of the Modbus driver. It maps the
// batteries the driver reaches (the RS-485 low-voltage BMS family, the SunSpec
// storage models and the dongle-family battery units) to the resources,
// relations and facts of the storage pack, in the units the pack declares for
// each element. It reads native evaluations that other packages decode and
// sends no request except through the on-demand native status read, which
// publishes nothing. Every native value without an element stays readable in
// the driver namespace.
package storage
