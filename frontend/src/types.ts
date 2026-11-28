export type SearchMode = 'hybrid' | 'text' | 'semantic';
export type SortMode = 'newest' | 'oldest' | 'most-liked' | 'least-liked' | 'most-saved' | 'random';

export interface Cluster {
  id: number;
  name: string;
  slug: string;
  color: string;
  description: string;
}

export interface Author {
  id: string;
  username: string;
  avatarUrl?: string;
  email?: string;
  bio?: string;
  createdAt?: string;
  jokesCount?: number;
  totalLikes?: number;
}

export interface Joke {
  id: string;
  text: string;
  triggers: string[];
  cluster: Cluster;
  author: Author;
  likesCount: number;
  favoritesCount: number;
  createdAt: string;
  posX: number;
  posY: number;
  posZ: number;
  liked?: boolean;
  favorited?: boolean;
  isDuplicate?: boolean;
  relevance?: number;
}

export interface JokeSearchResponse {
  keyword_results: Joke[];
  semantic_results: Joke[];
  results?: Joke[];
  query: string;
  mode: SearchMode;
  semantic_fallback?: boolean;
}
