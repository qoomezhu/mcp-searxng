import { LogLevel, log } from "./logging.js";

export interface SearXNGResult {
  url: string;
  title: string;
  content?: string;
  publishedDate?: string;
  img_src?: string;
  engine: string;
  score: number;
  category: string;
}

interface SearXNGWeb {
  results: SearXNGResult[];
}

export interface SearchParams {
  query: string;
  language?: string;
  time_range?: string;
  safesearch?: string;
  pageno?: number;
}

function createJSONError(responseText: string, context: { url: string }): Error {
  const preview = responseText.length > 200 
    ? responseText.substring(0, 200) + '...' 
    : responseText;
  return new Error(
    `Failed to parse JSON response from SearXNG.\n` +
    `URL: ${context.url}\n` +
    `Response preview: ${preview}`
  );
}

export async function searxngWebSearch(
  searxngUrl: string,
  params: SearchParams
): Promise<SearXNGResult[]> {
  const url = new URL("/search", searxngUrl);
  url.searchParams.set("q", params.query);
  url.searchParams.set("format", "json");

  if (params.language) url.searchParams.set("language", params.language);
  if (params.time_range) url.searchParams.set("time_range", params.time_range);
  if (params.safesearch) url.searchParams.set("safesearch", params.safesearch);
  if (params.pageno) url.searchParams.set("pageno", params.pageno.toString());

  log(LogLevel.INFO, `🔍 Searching: ${params.query}`);

  const response = await fetch(url.toString(), {
    headers: { Accept: "application/json" },
  });

  if (!response.ok) {
    throw new Error(`SearXNG search failed: \({response.status} \){response.statusText}`);
  }

  // ✅ 修复：先读取文本，再解析 JSON
  const responseText = await response.text();
  let data: SearXNGWeb;
  try {
    data = JSON.parse(responseText) as SearXNGWeb;
  } catch {
    throw createJSONError(responseText, { url: url.toString() });
  }

  log(LogLevel.INFO, `📊 Found ${data.results?.length || 0} results`);

  return data.results || [];
}
