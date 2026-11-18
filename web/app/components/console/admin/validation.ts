/** Shared validation helpers for the platform admin forms. */

const DNS_LABEL = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/;
const QUANTITY = /^\d+(\.\d+)?(m|k|Ki|Mi|Gi|Ti|K|M|G|T)?$/;

/** Validates a resource name (lowercase letters, digits and dashes). Returns an error or "". */
export function validateName(v: string, what = "名称"): string {
  const s = v.trim();
  if (!s) return `请填写${what}`;
  if (s.length > 63) return `${what}最多 63 个字符`;
  if (!DNS_LABEL.test(s)) return `${what}只能包含小写字母、数字和短横线，且以字母或数字开头和结尾`;
  return "";
}

/** Validates a Kubernetes-style quantity such as "8", "500m", "16Gi". */
export function validateQuantity(v: string, example: string): string {
  const s = v.trim();
  if (!s) return "请填写";
  if (!QUANTITY.test(s)) return `格式不正确，例如 ${example}`;
  return "";
}

export function validatePositiveInt(v: string): string {
  const s = v.trim();
  if (!s) return "请填写";
  if (!/^\d+$/.test(s) || parseInt(s, 10) < 1) return "请输入正整数";
  return "";
}

export function validateEmail(v: string): string {
  const s = v.trim();
  if (!s) return "请填写邮箱";
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(s)) return "邮箱格式不正确";
  return "";
}
