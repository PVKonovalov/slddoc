package slddoc

import "maps"

// The CIM class table the tests run with: sld-editor's cim section and the element classes of its symbols.
var testShapes = map[string]Class{
	"3": ClassRectangle, "7": ClassJunctionPoint, "14": ClassNonIntersection,
	"24": ClassBusBarSection, "41": ClassBreaker, "43": ClassBreaker, "42": ClassLoadBreakSwitch, "162": ClassDisconnector,
	"399": ClassPowerCircuitBreaker, "164": ClassSectionalizer, "163": ClassShortCircuiterNoGround,
	"166": ClassDisconnectorFuse, "44": ClassKnifeSwitch, "175": ClassKnifeSwitch3, "49": ClassDisconnector,
	"50": ClassSectionalizer, "51": ClassChassis, "54": ClassGroundSwitch, "398": ClassShortCircuiter,
	"76": ClassStarter, "31": ClassGround, "47": ClassPowerTransformer, "6": ClassBooster, "173": ClassGenerator,
	"174": ClassSynchronousCompensator, "39": ClassSynchronousMotor, "34": ClassCurrentTransformer,
	"55": ClassVoltageTransformer, "33": ClassChokeCoil, "37": ClassReactor, "397": ClassReactorShunt,
	"35": ClassSurgeArrester, "29": ClassSurgeArrester, "168": ClassSurgeArrester, "203": ClassFuse, "154": ClassFuse,
	"388": ClassCapacitor, "156": ClassResistor, "172": ClassCapacitorBank, "52": ClassHalfChassis,
	"389": ClassBlockingFilter, "320003": ClassFaultPassageIndicator, "330001": ClassLoad,
	"330002": ClassNetworkInfeed, "330003": ClassStaticVarCompensator,
}

var testExportClasses = map[string][]string{
	"24": {"BusbarSection"}, "41": {"Breaker", "DisconnectingCircuitBreaker"},
	"43": {"Breaker", "DisconnectingCircuitBreaker"}, "42": {"LoadBreakSwitch"}, "162": {"Disconnector"},
	"399": {"Breaker"}, "164": {"Switch"}, "163": {"Switch"}, "166": {"Fuse"}, "44": {"Disconnector"},
	"175": {"Disconnector"}, "49": {"Disconnector"}, "50": {"Switch"}, "51": {"Switch"}, "54": {"GroundDisconnector"},
	"398": {"GroundDisconnector"}, "76": {"Switch", "LoadBreakSwitch"}, "31": {"Ground"}, "47": {"PowerTransformer"},
	"6": {"PowerTransformer"}, "173": {"SynchronousMachine", "AsynchronousMachine"}, "174": {"SynchronousMachine"},
	"39": {"SynchronousMachine", "AsynchronousMachine"}, "34": {"CurrentTransformer"}, "55": {"PotentialTransformer"},
	"33": {"SeriesCompensator"}, "37": {"SeriesCompensator"}, "397": {"LinearShuntCompensator", "PetersenCoil"},
	"35": {"SurgeArrester"}, "29": {"SurgeArrester"}, "168": {"SurgeArrester"}, "203": {"Fuse"}, "154": {"Fuse"},
	"388": {"LinearShuntCompensator"}, "156": {"SeriesCompensator"}, "172": {"LinearShuntCompensator"},
	"52": {"Switch"}, "389": {"WaveTrap"}, "320003": {"FaultIndicator"},
	"330001": {"EnergyConsumer", "ConformLoad", "NonConformLoad"},
	"330002": {"ExternalNetworkInjection", "EquivalentInjection", "EnergySource"},
	"330003": {"StaticVarCompensator"}, LinkToObjectCode: {"EnergyConsumer", "EnergySource"},
}

var testImportShapes = map[string]string{
	"Breaker": "41", "DisconnectingCircuitBreaker": "41", "Disconnector": "162", "LoadBreakSwitch": "42",
	"GroundDisconnector": "54", "Fuse": "203", "Switch": "164", "Ground": "31", "SynchronousMachine": "173",
	"SynchronousMachine.condenser": "174", "SynchronousMachine.motor": "39", "AsynchronousMachine": "39",
	"AsynchronousMachine.generator": "173", "SeriesCompensator": "37", "LinearShuntCompensator": "172",
	"LinearShuntCompensator.reactor": "397", "NonlinearShuntCompensator": "172", "NonlinearShuntCompensator.reactor": "397",
	"PetersenCoil": "397", "BusbarSection": "24", "PowerTransformer": "47", "CurrentTransformer": "34",
	"PotentialTransformer": "55", "SurgeArrester": "168", "FaultIndicator": "320003", "WaveTrap": "389", "Junction": "7",
	"EnergyConsumer": "330001", "ConformLoad": "330001", "NonConformLoad": "330001", "ExternalNetworkInjection": "330002",
	"EquivalentInjection": "330002", "EnergySource": "330002", "StaticVarCompensator": "330003",
}

var testCIMTable = func() CIMClassTable {
	t, _, err := NewCIMClassTable(testShapes, testExportClasses, testImportShapes)
	if err != nil {
		panic(err)
	}
	return t
}()

// testImport is opt with the test table when it has none, and the test shapes under its own.
func testImport(opt CIMImportOptions) CIMImportOptions {
	classes := map[string]Class{}
	maps.Copy(classes, testShapes)
	maps.Copy(classes, opt.Classes)
	opt.Classes = classes
	if opt.CIMClasses.export == nil {
		opt.CIMClasses = testCIMTable
	}
	return opt
}

// testExport is opt with the test table when it has none.
func testExport(opt CIMOptions) CIMOptions {
	if opt.Classes.export == nil {
		opt.Classes = testCIMTable
	}
	return opt
}
