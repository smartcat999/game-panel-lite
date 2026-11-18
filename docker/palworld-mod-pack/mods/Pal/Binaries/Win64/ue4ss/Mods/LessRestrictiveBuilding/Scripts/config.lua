-- made by caites
local palConfig = {

    -- Change those for new mod or different version
    palModName = "Less Restrictive Building",
    palModVersion = "1.20",
    palGameVersion = "0.4.11.0",

    -- Those are generic for every mod
    clientRestarted = false,
    serverAcknowledgedPossession = false,
    palRedColorWrapperFront = "\27[31m",
    palRedColorWrapperBack = "\27[0m\n",

    -- Minimal distance between bases
	--BaseCampNeighborMinimumDistance = -8400,

	-- Maximum number of objects build by character
	PlayerRecord_BuildingObjectMaxNum = 99999999,

	-- Height at what floating foundations allowed to be built
	BuildSimulationFoundationFloatingAllowance = 99999999,

	-- Allowed inclination for objects, default is around 30
	BuildSimulationLeanAngleMax = 90,

	-- Max height allowed for building
	BuildingMaxZ = 99999999

}
return palConfig