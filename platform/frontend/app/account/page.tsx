import { AppShell } from "@/components/app-shell";
import { PageHeader } from "@/features/prototype/workspace-pages";

export default function AccountPage() {
  return <AppShell area="workspace" workspaceSlug="ember"><PageHeader title="账户设置" /><section className="detail-panel"><div className="compact-definition-grid"><div className="review-item"><span>用户</span><strong>Peng Wu</strong></div><div className="review-item"><span>登录方式</span><strong>GitHub · pengwu</strong></div><div className="review-item"><span>本地密码</span><strong>未设置</strong></div></div></section></AppShell>;
}
