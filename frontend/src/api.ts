import type { Joke, JokeSearchResponse, SearchMode } from './types';
import { rankLocally } from './data';

export class ApiError extends Error {
  constructor(message: string, readonly status: number) {
    super(message);
    this.name = 'ApiError';
  }
}

async function getJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, { ...init, headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) } });
  let payload: any;
  try { payload = await response.json(); } catch { payload = null; }
  if (!response.ok) {
    throw new ApiError(payload?.error ?? `Сервер вернул ошибку (${response.status}).`, response.status);
  }
  return payload as T;
}

export async function loadJokes(page = 1, filters: Record<string, string> = {}): Promise<{ jokes: Joke[]; hasMore: boolean }> {
  const query = new URLSearchParams({ page: String(page), limit: '12', ...filters });
  try {
    const result = await getJSON<{ jokes: Joke[]; has_more?: boolean }>(`/api/jokes?${query}`);
    return { jokes: result.jokes, hasMore: result.has_more ?? result.jokes.length === 12 };
  } catch {
    return { jokes: [], hasMore: false };
  }
}

export async function searchJokes(query: string, mode: SearchMode, source: Joke[] = []): Promise<JokeSearchResponse> {
  try {
    return await getJSON<JokeSearchResponse>(`/api/search?q=${encodeURIComponent(query)}&mode=${mode}`);
  } catch {
    const result = rankLocally(query, source, mode);
    return {
      keyword_results: result.keyword,
      semantic_results: result.semantic,
      results: [...result.keyword, ...result.semantic],
      query,
      mode,
    };
  }
}

export async function analyzeJoke(text: string): Promise<{ triggers: string[]; cluster: string }> {
  return getJSON('/api/jokes/analyze', { method: 'POST', body: JSON.stringify({ text }) });
}

export async function submitJoke(text: string, cluster: string, triggers: string[]): Promise<Joke> {
  return getJSON('/api/jokes', { method: 'POST', body: JSON.stringify({ text, cluster, triggers }) });
}

export async function toggleJokeAction(id: string, action: 'like' | 'favorite'): Promise<void> {
  await getJSON(`/api/jokes/${encodeURIComponent(id)}/${action}`, { method: 'POST', body: '{}' });
}

export async function suggestJokes(story: string): Promise<{ themes: string[]; jokes: Joke[] }> {
  return getJSON('/api/standup/suggest-jokes', { method: 'POST', body: JSON.stringify({ story }) });
}
