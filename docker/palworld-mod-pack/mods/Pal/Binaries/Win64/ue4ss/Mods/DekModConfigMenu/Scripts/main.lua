---
-- DekModConfigMenu 
-- Author: dekitarpg@gmail.com
-- Latest: https://www.nexusmods.com/palworld/mods/577
-- This "mod" provides an all important helper function to allow
-- .pak file logic mods to scan local directories for files.
-- THIS IS REQUIRED BY THE MOD CONFIG MENU TO WORK PROPERLY!
---
require 'os'

---
-- Helper functions for logging / errors
---
local LOG_PREFIX = "[DekModConfigMenu] "
local function Log(message) print(LOG_PREFIX .. message .. "\n") end
local function Error(message) error(LOG_PREFIX .. message .. "\n") end
local function LogWithHookIDs(Prefix, Suffix, preID, postID)
    Log(Prefix .. "(" .. preID .. ", " .. postID .. ")" .. " With CB: " .. Suffix)
end -- Helper function to log a message with the pre and post hook ID's


---
-- LOG_EXTRA_HOOK_DATA
-- Set to true to log extra data for debugging
--- 
local LOG_EXTRA_HOOK_DATA = false

---
-- REGISTERED_HOOKS
-- A table to store registered callback hooks so we can unregister them later
---
local REGISTERED_HOOKS = {}

---
-- CreateModsDirectory()
-- Creates a `~mods` directory within the Paks folder if it doesn't already exist
---
local function CreateModsDirectory()
    -- get the game directory tree as a table 
    local Dirs = IterateGameDirectories();
    if not Dirs then Error("Issue scanning directories!") end
    local PakModsDir = Dirs.Game.Content.Paks["~mods"]
    if not PakModsDir then
        Log("~mods folder doesnt exist, creating...")
        local path = Dirs.Game.Content.Paks.__absolute_path
        local modspath = path  .. "\\~mods"
        os.execute("mkdir \"" .. modspath .. "\"")
        Log("Created ~mods folder at: " .. modspath)
    end
    return PakModsDir
end

ExecuteWithDelay(1, CreateModsDirectory)

---
-- GetModDirectories() 
-- Returns the games LogicMods and LuaMods directories
--- 
local function GetModDirectories() 
    -- get the game directory tree as a table 
    local Dirs = IterateGameDirectories();
    if not Dirs then Error("Issue scanning directories!") end
    -- scan for logic mods folder
    -- throw error if not found
    local LogicModsDir = Dirs.Game.Content.Paks.LogicMods
    if not LogicModsDir then Error("Unable to find Content/Paks/LogicMods directory!") end
    -- scan for pak mods folder
    local PakModsDir = Dirs.Game.Content.Paks["~mods"]
    if not PakModsDir then Error("Unable to find Content/Paks/~mods directory!") end
    -- scan for lua mods folder
    -- throw error if not found
    local LuaModsDir
    local LuaModsRoot
    if Dirs.Game.Binaries.WinGDK then
        Log("Reading LUA Mod config from: WinGDK!") 
        LuaModsRoot = Dirs.Game.Binaries.WinGDK
    elseif Dirs.Game.Binaries.Win64 then
        Log("Reading LUA Mod config from: Win64!") 
        LuaModsRoot = Dirs.Game.Binaries.Win64
    end
    if LuaModsRoot.ue4ss then 
        Log("Reading LUA Mod config from: ue4ss subfolder (latest exp release!") 
        LuaModsDir = LuaModsRoot.ue4ss.Mods
    else 
        LuaModsDir = LuaModsRoot.Mods
    end
    if not LuaModsDir then Error("Unable to find LUA Mods directory!") end
    -- return the mod directories
    return LogicModsDir, LuaModsDir, PakModsDir
end

---
-- CommonBindModConfigLogic
-- This function binds a blueprint function to be called when an instance of WBP_ModConfigMenuUI_C
-- is created and the AfterClickResetButton or AfterClickSavedButton is called.
---
local function CommonBindModConfigLogic(HookPath, ParamContext, InModNameID, InCallbackFunctionName)
    if not InCallbackFunctionName then return end -- dont error as we use this to ignore AfterUpdatedConfig if not provided
    local ClassPath = "/Game/Mods/DekModConfigMenu_P/Widgets/WBP_ModConfigMenuUI"
    local InstancePath = ClassPath .. ".WBP_ModConfigMenuUI_C"
    local FunctionPath = InstancePath .. ":" .. HookPath
    local CallingClass = ParamContext:get()
    local CallingClassName = CallingClass:GetFName():ToString() 
    local CBFunctionName = InCallbackFunctionName:get():ToString()
    local CBFunction = CallingClass[CBFunctionName] -- try to find cb function from calling class
    if not CBFunction then Error("Unable to find function: " .. CBFunctionName) end
    Log(CallingClassName .. ":" .. CBFunctionName .." found!")
    -- store mod id as string and create a unique ID for this hook
    local ModNameString = InModNameID:get():ToString()
    local UniqueModHookID = ModNameString .. ":" .. CBFunctionName
    -- log a bunch of extra data for debugging if required
    if LOG_EXTRA_HOOK_DATA then 
        Log("-ClassPath: " .. ClassPath)
        Log("-InstancePath: " .. InstancePath)
        Log("-FunctionPath: " .. FunctionPath)
        Log("-ModNameString: " .. ModNameString)
        Log("-UniqueModHookID: " .. UniqueModHookID)
    end
    -- if we've already registered this hook, unregister it first
    if REGISTERED_HOOKS[UniqueModHookID] then 
        local preID, postID = table.unpack(REGISTERED_HOOKS[UniqueModHookID])
        LogWithHookIDs("-UnregisteringHook", UniqueModHookID, preID, postID)
        UnregisterHook(FunctionPath, preID, postID)
    end
    Log("-RegisteringHook", UniqueModHookID)
    -- actually register the hook so we cal trigger the callback when required
    local preID, postID = RegisterHook(FunctionPath, function(Context, ModName, ModJSONString)
        -- extract the ModID from ModName (entire mod filename)
        local ModID = ModName:get():ToString():match(".+/(.-)%.modconfig%.json$")
        -- if the mod name doesn't match the one we're looking for, ignore it
        -- Log("Checking for: " .. ModNameString .. " == " .. ModID)
        if ModNameString ~= ModID then return end 
        -- callback the desired function with the mod configs updated JSON string
        CBFunction(CallingClass, ModJSONString:get())
        -- just log that we've called the callback
        Log(HookPath .. " -> " .. UniqueModHookID)
    end)
    -- store the hook ID's so we can unregister it later if required
    -- this generally happens when the game is initially loading, 
    -- and when the player returns to the main menu after being in a game
    REGISTERED_HOOKS[UniqueModHookID] = {preID, postID}
    -- just log that we've registered the hook
    LogWithHookIDs("-RegisteredHook", UniqueModHookID, preID, postID)
end

---
-- Register the custom event to scan for config files
---
RegisterCustomEvent("DekScanModDirectories", function(ParamContext, OutArrayParam)
    -- Ensure OutArrayParam is of type TArray
    local OutArray = OutArrayParam:get()
    if OutArray:type() ~= "TArray" then 
        Error("DekScanModDirectories OutParam#1 must be TArray but was " .. OutArray:type()) 
    end
    -- Load configurations for mods.
    LogicModsDir, LuaModsDir, PakModsDir = GetModDirectories()
    local NoModError = "[DekScanDirectory] Issue locating mod directories.\n"
    if not LogicModsDir then Error(NoModError) end
    if not LuaModsDir then Error(NoModError) end
    -- store the mod config file paths found
    local ConfigurableMods = {}
    -- iterate over each lua mods folder to search for its modconfig file
    for _, ModFolder in pairs(LuaModsDir) do
        local FolderName = ModFolder.__name
        Log("FOUND ModFolder: " .. FolderName)
        -- iterate over each mod folders inner files 
        -- normally only enabled.txt, and a Scripts folder with main.lua file
        -- but we only care about .modconfig.json files 
        for _, PotentialConfigFile in pairs(ModFolder.__files) do
            local FileName = PotentialConfigFile.__name
            if string.find(FileName, "modconfig.json") then
                Log("FOUND LUA MOD CONFIG: " .. FileName)
                table.insert(ConfigurableMods, PotentialConfigFile.__absolute_path)
            else
                Log("IGNORING: Mods/" .. FolderName .. "/" .. FileName)
            end
        end
    end
    -- iterate over logic mod folder to search for modconfig files
    for _, PotentialConfigFile in pairs(LogicModsDir.__files) do
        local FileName = PotentialConfigFile.__name
        if string.find(FileName, "modconfig.json") then
            Log("FOUND BP MOD CONFIG: " .. FileName)
            table.insert(ConfigurableMods, PotentialConfigFile.__absolute_path)
        else
            Log("IGNORING: LogicMods/" .. FileName)
        end
    end
    -- update OutArrayParam to list the scanned file paths
    OutArrayParam:get():ForEach(function(i, v)
        if ConfigurableMods[i] then 
            v:set(ConfigurableMods[i]) 
            Log("ADDED: " .. ConfigurableMods[i])
        end
    end)
end)

--- 
-- DekBindToNotifyOnNewObject
-- Binds blueprint callback functions to AfterClickResetButton and AfterClickSavedButton
-- REMOVED FOR NOW AS KEPT CAUSING CRASH ISSUES
--- 
-- RegisterCustomEvent("SetupModConfigBindings", function(ParamContext, InModNameID, InOnSavedCallbackFunctionName, InOnUpdatedCallbackFunctionName)
--     Log("-RegisteringCustomEvent:SetupModConfigBindings", InModNameID:get():ToString())
--     CommonBindModConfigLogic("AfterClickSavedButton", ParamContext, InModNameID, InOnSavedCallbackFunctionName)
--     CommonBindModConfigLogic("AfterUpdatedConfig", ParamContext, InModNameID, InOnUpdatedCallbackFunctionName) 
-- end)

-- ExecuteInGameThread(function()
--     ExecuteWithDelay(1000, function()
--     end)
-- end)



---
-- DekCheckModFileExists
-- This function checks if a file exists in the mod directories
---
RegisterCustomEvent("DekCheckModFileExists", function(ParamContext, InModDirType, InModNameID, OutBoolParam)
    local ModDirType = string.lower(InModDirType:get():ToString())
    local ModNameID = InModNameID:get():ToString()
    LogicModsDir, LuaModsDir, PakModsDir = GetModDirectories()
    local NoModError = "[DekCheckModFileExists] Issue locating mod directories.\n"
    if not LogicModsDir then Error(NoModError) end
    if not LuaModsDir then Error(NoModError) end
    if not PakModsDir then Error(NoModError) end
    local ModDir
    if ModDirType == "logicmods" or ModDirType == "logic" then ModDir = LogicModsDir
    elseif ModDirType == "luamods" or ModDirType == "lua" then ModDir = LuaModsDir
    elseif ModDirType == "pakmods" or ModDirType == "pak" then ModDir = PakModsDir
    else Error("Invalid ModDirType: " .. ModDirType) end
    -- check if the mod file exists
    local ModSeemsExisty = false
    Log("Checking for: " .. ModNameID .. " in " .. ModDirType)
    -- check the subfolders within the desired folder (lua mods)
    for _, PotentialSubfolder in pairs(ModDir) do
        local FolderName = PotentialSubfolder.__name
        if string.find(FolderName, ModNameID) then
            ModSeemsExisty = true
            break
        end
    end
    if not ModSeemsExisty then
        -- check the files within the desired folder (logic/pak mods)
        for _, PotentialModFile in pairs(ModDir.__files) do
            local FileName = PotentialModFile.__name
            if string.find(FileName, ModNameID) then
                ModSeemsExisty = true
                break
            end
        end
    end
    OutBoolParam:set(ModSeemsExisty)
end)

--- 
-- DekRegisterHook
-- Binds a callback function to a RegisterHook event for a specific class path
--- 
local function DekGetHookID(Context, InHookPath)
    local ClassName = Context:get():GetClass():GetFullName():ToString()
    local UniqueHookID = ClassName .. ":" .. InHookPath:get():ToString()
    return UniqueHookID
end 

RegisterCustomEvent("DekRegisterHook", function(ParamContext, InClassPathToNotify, InCallbackFunctionName)
    local CallingClass = ParamContext:get()
    local ClassPath = InClassPathToNotify:get()
    if ClassPath:type() ~= "FString" then error(string.format("DekRegisterHook Param #1 must be FString but was %s", ClassPath:type())) end
    local CBFunctionName = InCallbackFunctionName:get()
    if CBFunctionName:type() ~= "FString" then error(string.format("DekRegisterHook Param #2 must be FString but was %s", CBFunctionName:type())) end
    local CBFunction = CallingClass[CBFunctionName:ToString()]
    local UniqueHookID = DekGetHookID(ParamContext, InClassPathToNotify)

    Log("-CallingClass: " .. CallingClass:GetFullName())
    Log("-ClassPath: " .. ClassPath:ToString())
    Log("-CBFunctionName: " .. CBFunctionName:ToString())
    Log("-CBFunction: obtained!")

    Log("CALLING: DekRegisterHook : " .. UniqueHookID)
    if REGISTERED_HOOKS[UniqueHookID] then 
        local preID, postID = table.unpack(REGISTERED_HOOKS[UniqueHookID])
        Log("Unregistering Hook: " .. UniqueHookID)
        UnregisterHook(ClassPath:ToString(), preID, postID)
    end
    local preID, postID = RegisterHook(ClassPath:ToString(), function(Context)
        -- Log("CALLING: RegisteredHook" .. UniqueHookID)
        CBFunction(ParamContext, Context:get())
    end)
    REGISTERED_HOOKS[UniqueHookID] = {preID, postID}    
    Log("#HOOKS: " .. #REGISTERED_HOOKS)
end)

RegisterCustomEvent("DekUnregisterHook", function(ParamContext, InClassPathToNotify)
    local ClassPath = InClassPathToNotify:get()
    if ClassPath:type() ~= "FString" then error(string.format("DekUnregisterHook Param #1 must be FString but was %s", ClassPath:type())) end
    local UniqueHookID = DekGetHookID(ParamContext, InClassPathToNotify)
    if REGISTERED_HOOKS[UniqueHookID] then 
        Log("Unregistering Hook: " .. UniqueHookID)
        local preID, postID = table.unpack(REGISTERED_HOOKS[UniqueHookID])
        UnregisterHook(ClassPath:ToString(), preID, postID)
        REGISTERED_HOOKS[UniqueHookID] = nil
    end
end)

--- End Of DekModConfigMenu Blueprint Function Library -
-- by dekitarpg@gmail.com
--- 
