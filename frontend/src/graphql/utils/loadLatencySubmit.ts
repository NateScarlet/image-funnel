import { print } from "graphql";
import { AppendLoadLatencySegmentDocument } from "@/graphql/generated";
import mutate from "./mutate";
import randomUUID from "@/utils/randomUUID";

function variables(runId: string, segment: string) {
  return { input: { runId, segment, clientMutationId: randomUUID() } };
}

/** 提交一段归因记录，返回服务端写出的绝对路径 */
export async function submitLoadLatencySegment(runId: string, segment: string): Promise<string> {
  const { data } = await mutate(AppendLoadLatencySegmentDocument, {
    variables: variables(runId, segment),
  });
  if (!data) {
    throw new Error("appendLoadLatencySegment returned no data");
  }
  return data.appendLoadLatencySegment.path;
}

/**
 * 页面隐藏时无法等待 Promise，用 sendBeacon 投递同一条 mutation。
 * 同源请求自动带 Cookie，与 Apollo 的 credentials: include 一致。
 */
export function beaconLoadLatencySegment(runId: string, segment: string): boolean {
  if (typeof navigator === "undefined" || typeof navigator.sendBeacon !== "function") {
    return false;
  }
  const body = JSON.stringify({
    query: print(AppendLoadLatencySegmentDocument),
    variables: variables(runId, segment),
  });
  return navigator.sendBeacon(
    new URL("graphql", document.baseURI).href,
    new Blob([body], { type: "application/json" }),
  );
}
