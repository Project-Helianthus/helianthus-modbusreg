// Package pv is the photovoltaic producer of the Modbus driver. It maps the
// SunSpec inverters and the dongle-family (Huawei) inverters the driver
// acquires to the resources, relations and facts of the PV pack, in SI units.
// It reads native evaluations that other packages decode and sends no request.
// Every native value stays readable in the driver namespace beside the facts,
// whether or not an element of the pack takes it.
package pv
