import { describe, expect, it } from "vitest";
import { classifyRuntimeError, formatCreateServerError } from "./runtime-errors";
import type { MessageKey } from "./i18n";

const t = (key: MessageKey, params?: Record<string, string | number>) => {
  if (params?.field) {
    return `${key}:${params.field}`;
  }
  return key;
};

describe("runtime error display", () => {
  it("classifies runtime install and create failures into localizable messages", () => {
    expect(classifyRuntimeError("server runtime is not installed; install it from Game Library first")).toBe("runtimeInstallIncomplete");
    expect(classifyRuntimeError("runtime image archive is empty")).toBe("runtimeInstallIncomplete");
    expect(classifyRuntimeError("pull access denied for smartcat99999/dst-server, repository does not exist")).toBe("runtimeImageUnavailable");
    expect(classifyRuntimeError("manifest unknown: manifest unknown")).toBe("runtimeImageUnavailable");
    expect(classifyRuntimeError("Don't Starve Together server runtime is currently supported only on amd64 Docker hosts")).toBe(
      "runtimeUnsupportedArchitecture"
    );
    expect(classifyRuntimeError("Docker runtime unavailable: cannot connect to Docker daemon")).toBe("runtimeDockerUnavailable");
    expect(classifyRuntimeError("Bind for 0.0.0.0:7778 failed: port is already allocated")).toBe("runtimePortAlreadyUsed");
  });

  it("keeps form validation errors product-specific in the create flow", () => {
    expect(formatCreateServerError(new Error("admin password is required"), t)).toBe("requiredFieldError:adminPassword");
    expect(formatCreateServerError(new Error("eula must be accepted"), t)).toBe("requiredAgreementError:minecraftEulaAccepted");
  });
});
