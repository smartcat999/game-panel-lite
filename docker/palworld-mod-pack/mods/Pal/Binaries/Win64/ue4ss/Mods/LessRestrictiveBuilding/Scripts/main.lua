-- Author: Caites

local palConfig = require "config"

-- Prints the mod successful loading message in red
print(palConfig.palRedColorWrapperFront .. palConfig.palModName .. " version " .. palConfig.palModVersion .. " loaded for game version " .. palConfig.palGameVersion .. palConfig.palRedColorWrapperBack)

-- [CLIENT] Register the new modifiers and print a successful change message in red
RegisterHook("/Script/Engine.PlayerController:ClientRestart", function()
    if not palConfig.clientRestarted then
        local items = FindAllOf("PalGameSetting")
        if items then
            for _, item in ipairs(items) do                
				item.BuildSimulationFoundationFloatingAllowance = palConfig.BuildSimulationFoundationFloatingAllowance
				item.BuildSimulationLeanAngleMax = palConfig.BuildSimulationLeanAngleMax
				item.BuildingMaxZ = palConfig.BuildingMaxZ
				--item.BaseCampNeighborMinimumDistance = palConfig.BaseCampNeighborMinimumDistance
				item.PlayerRecord_BuildingObjectMaxNum = palConfig.PlayerRecord_BuildingObjectMaxNum
            end
        end
        print(palConfig.palRedColorWrapperFront .. "[" .. palConfig.palModName .. "] Floating Allowance Height set to " .. palConfig.BuildSimulationFoundationFloatingAllowance .. palConfig.palRedColorWrapperBack)
		print(palConfig.palRedColorWrapperFront .. "[" .. palConfig.palModName .. "] Building Lean Angle set to " .. palConfig.BuildSimulationLeanAngleMax .. palConfig.palRedColorWrapperBack)
		print(palConfig.palRedColorWrapperFront .. "[" .. palConfig.palModName .. "] Building Max Height set to " .. palConfig.BuildingMaxZ .. palConfig.palRedColorWrapperBack)
		--print(palConfig.palRedColorWrapperFront .. "[" .. palConfig.palModName .. "] Min Distance to Neighbor Base set to " .. palConfig.BaseCampNeighborMinimumDistance .. palConfig.palRedColorWrapperBack)
		print(palConfig.palRedColorWrapperFront .. "[" .. palConfig.palModName .. "] Max Number of Buildings set to " .. palConfig.PlayerRecord_BuildingObjectMaxNum .. palConfig.palRedColorWrapperBack)
    end
    palConfig.clientRestarted = 1
end)

--Author: XxInvictus

-- [DEDICATED SRV] Register the new modifiers and print a successful change message in red
RegisterHook("/Script/Engine.PlayerController:ServerAcknowledgePossession", function(Context)
    if not palConfig.serverAcknowledgedPossession then
        local items = FindAllOf("PalGameSetting")
        if items then
            for _, item in ipairs(items) do                
				item.BuildSimulationFoundationFloatingAllowance = palConfig.BuildSimulationFoundationFloatingAllowance
				item.BuildSimulationLeanAngleMax = palConfig.BuildSimulationLeanAngleMax
				item.BuildingMaxZ = palConfig.BuildingMaxZ
				--item.BaseCampNeighborMinimumDistance = palConfig.BaseCampNeighborMinimumDistance
				item.PlayerRecord_BuildingObjectMaxNum = palConfig.PlayerRecord_BuildingObjectMaxNum
            end
        end
        print(palConfig.palRedColorWrapperFront .. "[" .. palConfig.palModName .. "] Floating Allowance Height set to " .. palConfig.BuildSimulationFoundationFloatingAllowance .. palConfig.palRedColorWrapperBack)
		print(palConfig.palRedColorWrapperFront .. "[" .. palConfig.palModName .. "] Building Lean Angle set to " .. palConfig.BuildSimulationLeanAngleMax .. palConfig.palRedColorWrapperBack)
		print(palConfig.palRedColorWrapperFront .. "[" .. palConfig.palModName .. "] Building Max Height set to " .. palConfig.BuildingMaxZ .. palConfig.palRedColorWrapperBack)
		--print(palConfig.palRedColorWrapperFront .. "[" .. palConfig.palModName .. "] Min Distance to Neighbor Base set to " .. palConfig.BaseCampNeighborMinimumDistance .. palConfig.palRedColorWrapperBack)
		print(palConfig.palRedColorWrapperFront .. "[" .. palConfig.palModName .. "] Max Number of Buildings set to " .. palConfig.PlayerRecord_BuildingObjectMaxNum .. palConfig.palRedColorWrapperBack)
    end
    palConfig.serverAcknowledgedPossession = true
end)

-- Author: yakuzadeso

local clientRestarted = false

local PalBuildObject = StaticFindObject("/Script/Pal.Default__PalBuildObject")

RegisterHook("/Script/Engine.PlayerController:ClientRestart", function()
	if not clientRestarted then
		if not PalBuildObject then
			PalBuildObject = StaticFindObject("/Script/Pal.Default__PalBuildObject")
		end
		PalBuildObject.bSpawnableIfOverlapped = 1
	end
	clientRestarted = true
end)

RegisterHook("/Script/Engine.PlayerController:ServerAcknowledgePossession", function()
	if not PalBuildObject then
		PalBuildObject = StaticFindObject("/Script/Pal.Default__PalBuildObject")
	end
	PalBuildObject.bSpawnableIfOverlapped = 1
end)