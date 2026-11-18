"use client";

import { Fragment } from "react";
import { ArrowUpRight } from "lucide-react";
import type { AuditEventItem } from "../../console/lib/types";
import {
  type NotificationDeliveryRecord,
  describeAuditAction,
  describeAuditMethodPath,
  describeAuditTargetBase,
  describeChannelType,
  formatAuditActor,
  formatAuditPathShort,
  formatAuditRole,
  formatDateTime,
  summarizeUserAgent,
} from "../../console/lib/observability";
import { Badge, CopyButton, focusRing, Modal, Mono } from "./ui";

function renderBreakablePath(uri: string) {
  const parts = uri.split("/");
  return parts.map((seg, idx) =>
    idx === 0 ? (
      <Fragment key={idx}>{seg}</Fragment>
    ) : (
      <Fragment key={idx}>
        <wbr />/{seg}
      </Fragment>
    ),
  );
}

export function AuditDetailModal({
  event,
  onClose,
  onTraceLogs,
}: {
  event: AuditEventItem | null;
  onClose: () => void;
  onTraceLogs?: (requestId: string) => void;
}) {
  if (!event) {
    return <Modal open={false} onClose={onClose} title="操作审计详情" width="max-w-[480px]" />;
  }

  const code = event.statusCode || 200;
  const tone = code >= 500 ? "danger" : code >= 400 ? "warning" : "success";
  const statusLabel =
    code === 403
      ? "403 拒绝"
      : code === 401
        ? "401 未授权"
        : code >= 400
          ? `${code} 异常`
          : `${code} 成功`;

  const methodPath = describeAuditMethodPath(event);
  const spaceIdx = methodPath.indexOf(" ");
  const httpMethod = spaceIdx > 0 ? methodPath.slice(0, spaceIdx) : "POST";
  const httpUri = spaceIdx > 0 ? methodPath.slice(spaceIdx + 1) : methodPath;
  const reqId = event.requestID || event.metadata.name;
  const rawUa = event.attributes?.userAgent || "GamePanel Console Client";
  const targetBase = describeAuditTargetBase(event);
  const roleLabel = formatAuditRole(event.attributes?.role);
  const targetName = event.targetName || event.resourceName || event.metadata.name || "";
  const filePath = formatAuditPathShort(event.attributes?.path, event.metadata.tenant, targetName);
  const hasDiff = Boolean(event.attributes?.diffBefore || event.attributes?.diffAfter);

  const title = (
    <span className="flex min-w-0 items-center gap-2.5">
      <span className="truncate">{describeAuditAction(event)}</span>
      <Badge tone={tone}>{statusLabel}</Badge>
    </span>
  );

  return (
    <Modal open={true} onClose={onClose} title={title} width="max-w-[480px]">
      <div className="pb-1">
        <dl className="divide-y divide-[#EEF0F7] text-[13px]">
          <div className="grid grid-cols-[76px_1fr] items-center gap-3 py-2.5">
            <dt className="text-[#5B6075]">目标资源</dt>
            <dd className="flex min-w-0 items-center gap-2 font-medium text-[#14161F]">
              <Mono className="truncate">{targetBase}</Mono>
              {roleLabel && <Badge tone="accent">{roleLabel}</Badge>}
            </dd>
          </div>

          {filePath && (
            <div className="grid grid-cols-[76px_1fr] items-center gap-3 py-2.5">
              <dt className="text-[#5B6075]">文件路径</dt>
              <dd className="min-w-0">
                <Mono className="block truncate text-xs text-[#3D4258]">{filePath}</Mono>
              </dd>
            </div>
          )}

          <div className="grid grid-cols-[76px_1fr] items-center gap-3 py-2.5">
            <dt className="text-[#5B6075]">操作主体</dt>
            <dd className="flex min-w-0 items-center gap-2">
              <span className="truncate font-medium text-[#14161F]">{formatAuditActor(event.actor)}</span>
              <span className="text-[#8A90A6]">·</span>
              <Mono className="shrink-0 text-xs text-[#5B6075]">{event.clientIP || "127.0.0.1"}</Mono>
            </dd>
          </div>

          <div className="grid grid-cols-[76px_1fr] items-center gap-3 py-2.5">
            <dt className="text-[#5B6075]">发生时间</dt>
            <dd className="font-mono text-[#14161F]">
              {formatDateTime(event.timestamp || event.metadata.creationTimestamp)}
            </dd>
          </div>

          <div className="grid grid-cols-[76px_1fr] items-center gap-3 py-2.5">
            <dt className="text-[#5B6075]">客户端</dt>
            <dd className="truncate text-[#14161F]" title={rawUa}>
              {summarizeUserAgent(rawUa)}
            </dd>
          </div>

          <div className="grid grid-cols-[76px_1fr] items-center gap-3 py-2.5">
            <dt className="text-[#5B6075]">请求 ID</dt>
            <dd className="flex min-w-0 items-center justify-between gap-2">
              <Mono className="truncate text-xs text-[#3D4258]">{reqId}</Mono>
              <div className="flex shrink-0 items-center gap-1">
                {onTraceLogs && event.requestID && (
                  <button
                    type="button"
                    onClick={() => onTraceLogs(event.requestID!)}
                    className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium text-[#2B3AE8] transition-colors hover:bg-[#EEF1FF] ${focusRing}`}
                  >
                    <ArrowUpRight className="h-3.5 w-3.5" />
                    追踪日志
                  </button>
                )}
                <CopyButton text={reqId} label="复制请求 ID" />
              </div>
            </dd>
          </div>
        </dl>

        <div className="mt-4">
          <div className="mb-1.5 text-xs text-[#5B6075]">请求接口</div>
          <div className="flex items-baseline gap-2.5 rounded-xl bg-[#F7F8FC] px-3.5 py-2.5 font-mono text-xs leading-relaxed">
            <span className="shrink-0 font-semibold text-[#2B3AE8]">{httpMethod}</span>
            <span className="min-w-0 break-words text-[#3D4258]">{renderBreakablePath(httpUri)}</span>
          </div>
        </div>

        {hasDiff && (
          <div className="mt-3.5">
            <div className="mb-1.5 text-xs text-[#5B6075]">变更内容</div>
            <div className="space-y-1.5 font-mono text-xs leading-relaxed">
              {event.attributes?.diffBefore && (
                <div className="break-words rounded-xl bg-[#FDECEC]/80 px-3.5 py-2 text-[#B42318]">
                  - {event.attributes.diffBefore}
                </div>
              )}
              {event.attributes?.diffAfter && (
                <div className="break-words rounded-xl bg-[#E8F8EF]/80 px-3.5 py-2 text-[#16794B]">
                  + {event.attributes.diffAfter}
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </Modal>
  );
}

export function NotificationDetailModal({
  delivery,
  onClose,
}: {
  delivery: NotificationDeliveryRecord | null;
  onClose: () => void;
}) {
  if (!delivery) {
    return <Modal open={false} onClose={onClose} title="通知详情" width="max-w-[480px]" />;
  }

  const tone =
    delivery.status === "delivered"
      ? "success"
      : delivery.status === "retrying"
        ? "warning"
        : "danger";
  const statusLabel =
    delivery.status === "delivered"
      ? "已送达"
      : delivery.status === "retrying"
        ? "重试中"
        : "发送失败";

  const title = (
    <span className="flex min-w-0 items-center gap-2.5">
      <span className="truncate">{delivery.channelName}</span>
      <Badge tone={tone}>{statusLabel}</Badge>
    </span>
  );

  return (
    <Modal open={true} onClose={onClose} title={title} width="max-w-[480px]">
      <div className="pb-1">
        <dl className="divide-y divide-[#EEF0F7] text-[13px]">
          <div className="grid grid-cols-[76px_1fr] items-center gap-3 py-2.5">
            <dt className="text-[#5B6075]">渠道类型</dt>
            <dd className="truncate font-medium text-[#14161F]">
              {describeChannelType(delivery.channelType)}
            </dd>
          </div>

          <div className="grid grid-cols-[76px_1fr] items-center gap-3 py-2.5">
            <dt className="text-[#5B6075]">发送时间</dt>
            <dd className="font-mono text-[#14161F]">{formatDateTime(delivery.timestamp)}</dd>
          </div>

          <div className="grid grid-cols-[76px_1fr] items-center gap-3 py-2.5">
            <dt className="text-[#5B6075]">投递尝试</dt>
            <dd className="flex min-w-0 items-center gap-2 text-[#14161F]">
              <span className="font-mono">共 {delivery.attempts} 次</span>
              {delivery.nextRetryAt && (
                <>
                  <span className="text-[#8A90A6]">·</span>
                  <span className="truncate font-mono text-xs text-[#B54708]">
                    下次重试 {formatDateTime(delivery.nextRetryAt)}
                  </span>
                </>
              )}
            </dd>
          </div>

          <div className="grid grid-cols-[76px_1fr] items-center gap-3 py-2.5">
            <dt className="text-[#5B6075]">记录 ID</dt>
            <dd className="flex min-w-0 items-center justify-between gap-2">
              <Mono className="truncate text-xs text-[#3D4258]">{delivery.deliveryId}</Mono>
              <CopyButton text={delivery.deliveryId} label="复制记录 ID" />
            </dd>
          </div>
        </dl>

        <div className="mt-4">
          <div className="mb-1.5 text-xs text-[#5B6075]">推送内容</div>
          <div className="space-y-1.5 rounded-xl bg-[#F7F8FC] px-3.5 py-3 text-[13px]">
            <div className="font-medium text-[#14161F]">{delivery.title}</div>
            {delivery.message && (
              <div className="whitespace-pre-wrap text-xs leading-relaxed text-[#3D4258]">
                {delivery.message}
              </div>
            )}
          </div>
        </div>

        {delivery.error && (
          <div className="mt-3.5">
            <div className="mb-1.5 text-xs text-[#B42318]">错误响应</div>
            <div className="break-words rounded-xl bg-[#FDECEC]/80 px-3.5 py-2.5 font-mono text-xs leading-relaxed text-[#B42318]">
              {delivery.error}
            </div>
          </div>
        )}
      </div>
    </Modal>
  );
}
