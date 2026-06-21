export const showWorldAndBackupFeatures = false;

export function isWorldOrBackupEventType(type: string) {
  return type.startsWith("world.") || type.startsWith("backup.") || type.startsWith("save.");
}
