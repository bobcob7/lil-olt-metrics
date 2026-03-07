import { ref } from 'vue';
import { promQuery, promRangeQuery, getVal } from '../api/prometheus';
import type { PromResult, PromRangeResult } from '../api/types';
import { STEP_MAP, RANGE_SECONDS } from './useTimeRange';

export interface ModelTokens {
  input: number;
  output: number;
  cacheRead: number;
  cacheCreation: number;
}

export interface DashboardData {
  totalTokens: number;
  totalCost: number;
  totalActive: number;
  totalSessions: number;
  totalCommits: number;
  totalPRs: number;
  linesAdded: number;
  linesRemoved: number;
  modelTokens: Record<string, ModelTokens>;
  costByModel: Record<string, number>;
  langCounts: Record<string, number>;
  toolCounts: Record<string, number>;
  totalAccept: number;
  totalReject: number;
  activeTimeBySession: Record<string, Record<string, number>>;
  activeTimeTypes: readonly string[];
  tokensRange: readonly PromRangeResult[];
  costRange: readonly PromRangeResult[];
  locRange: readonly PromRangeResult[];
}

const parseModelTokens = (tokens: readonly PromResult[]): Record<string, ModelTokens> => {
  const result: Record<string, ModelTokens> = {};
  for (const r of tokens) {
    const model = r.metric['model'] ?? 'unknown';
    const type = r.metric['type'] ?? 'unknown';
    if (!result[model]) result[model] = { input: 0, output: 0, cacheRead: 0, cacheCreation: 0 };
    const val = parseFloat(r.value[1] ?? '0');
    const mt = result[model]!;
    if (type === 'input') mt.input += val;
    else if (type === 'output') mt.output += val;
    else if (type === 'cacheRead') mt.cacheRead += val;
    else if (type === 'cacheCreation') mt.cacheCreation += val;
  }
  return result;
};

const parseCostByModel = (costs: readonly PromResult[]): Record<string, number> => {
  const result: Record<string, number> = {};
  for (const r of costs) {
    const model = r.metric['model'] ?? 'unknown';
    result[model] = (result[model] ?? 0) + parseFloat(r.value[1] ?? '0');
  }
  return result;
};

const parseEditDecisions = (editDecisions: readonly PromResult[]) => {
  const langCounts: Record<string, number> = {};
  const toolCounts: Record<string, number> = {};
  let totalAccept = 0;
  let totalReject = 0;
  for (const r of editDecisions) {
    const lang = r.metric['language'] ?? 'unknown';
    const tool = r.metric['tool_name'] ?? 'unknown';
    const decision = r.metric['decision'] ?? 'unknown';
    const val = parseFloat(r.value[1] ?? '0');
    langCounts[lang] = (langCounts[lang] ?? 0) + val;
    toolCounts[tool] = (toolCounts[tool] ?? 0) + val;
    if (decision === 'accept') totalAccept += val;
    else if (decision === 'reject') totalReject += val;
  }
  return { langCounts, toolCounts, totalAccept, totalReject };
};

const parseActiveTime = (activeTime: readonly PromResult[]) => {
  const sessionMap: Record<string, Record<string, number>> = {};
  const typeSet = new Set<string>();
  for (const r of activeTime) {
    const sid = r.metric['session_id'] ?? r.metric['sessionId'] ?? 'unknown';
    const type = r.metric['type'] ?? 'active';
    typeSet.add(type);
    if (!sessionMap[sid]) sessionMap[sid] = {};
    sessionMap[sid]![type] = (sessionMap[sid]![type] ?? 0) + parseFloat(r.value[1] ?? '0');
  }
  return { sessionMap, types: [...typeSet].sort() };
};

const parseLoc = (loc: readonly PromResult[]) => {
  let linesAdded = 0;
  let linesRemoved = 0;
  for (const r of loc) {
    const type = r.metric['type'] ?? 'unknown';
    const val = parseFloat(r.value[1] ?? '0');
    if (type === 'added') linesAdded += val;
    else if (type === 'removed') linesRemoved += val;
  }
  return { linesAdded, linesRemoved };
};

export const useDashboardData = () => {
  const data = ref<DashboardData | null>(null);
  const error = ref<string | null>(null);
  const lastUpdated = ref<string | null>(null);

  const refresh = async (range: string): Promise<void> => {
    try {
      const step = STEP_MAP[range] ?? '12h';
      const rangeSecs = RANGE_SECONDS[range] ?? 2592000;
      const end = Math.floor(Date.now() / 1000);
      const start = end - rangeSecs;

      const [tokens, costs, activeTime, sessions, commits, prs, loc, editDecisions] = await Promise.all([
        promQuery(`max_over_time(claude_code_token_usage_tokens_total[${range}])`),
        promQuery(`max_over_time(claude_code_cost_usage_USD_total[${range}])`),
        promQuery(`max_over_time(claude_code_active_time_total_seconds_total[${range}])`),
        promQuery(`max_over_time(claude_code_session_count_total[${range}])`),
        promQuery(`max_over_time(claude_code_commit_count_total[${range}])`),
        promQuery(`max_over_time(claude_code_pull_request_count_total[${range}])`),
        promQuery(`max_over_time(claude_code_lines_of_code_count_total[${range}])`),
        promQuery(`max_over_time(claude_code_code_edit_tool_decision_total[${range}])`),
      ]);

      const [tokensRange, costRange, locRange] = await Promise.all([
        promRangeQuery('sum by (type)(increase(claude_code_token_usage_tokens_total[{step}]))', start, end, step),
        promRangeQuery('sum by (model)(increase(claude_code_cost_usage_USD_total[{step}]))', start, end, step),
        promRangeQuery('sum by (type)(increase(claude_code_lines_of_code_count_total[{step}]))', start, end, step),
      ]);

      const modelTokens = parseModelTokens(tokens);
      const costByModel = parseCostByModel(costs);
      const { langCounts, toolCounts, totalAccept, totalReject } = parseEditDecisions(editDecisions);
      const { sessionMap, types: activeTimeTypes } = parseActiveTime(activeTime);
      const { linesAdded, linesRemoved } = parseLoc(loc);

      data.value = {
        totalTokens: getVal(tokens),
        totalCost: getVal(costs),
        totalActive: getVal(activeTime),
        totalSessions: getVal(sessions),
        totalCommits: getVal(commits),
        totalPRs: getVal(prs),
        linesAdded,
        linesRemoved,
        modelTokens,
        costByModel,
        langCounts,
        toolCounts,
        totalAccept,
        totalReject,
        activeTimeBySession: sessionMap,
        activeTimeTypes,
        tokensRange,
        costRange,
        locRange,
      };

      error.value = null;
      lastUpdated.value = 'Updated ' + new Date().toLocaleTimeString();
    } catch (err) {
      error.value = `Failed to fetch metrics: ${err instanceof Error ? err.message : String(err)}. Is lil-olt-metrics running?`;
    }
  };

  return { data, error, lastUpdated, refresh };
};
