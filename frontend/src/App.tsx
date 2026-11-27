import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { View, Text, Pressable } from 'react-native';
import {
  ArrowRight, ArrowUpRight, Bookmark, Check, ChevronDown, ChevronRight, CircleDollarSign,
  Clock3, Compass, Copy, CreditCard, FileText, Heart, Home, Info, LoaderCircle, LockKeyhole,
  Menu, Mic, Plus, Search, Share2, Shield, Sparkles, Star, Tags, ThumbsUp, Upload, UserRound,
  Wand2, X, Zap,
} from 'lucide-react';
const Galaxy = lazy(() => import('./Galaxy').then((module) => ({ default: module.Galaxy })));
import { analyzeJoke, searchJokes, suggestJokes, submitJoke, toggleJokeAction } from './api';
import { clusters, rankLocally } from './data';
import type { Author, Joke, JokeSearchResponse, SearchMode, SortMode } from './types';

const MODE_LABELS: Record<SearchMode, string> = {
  hybrid: 'Гибридный',
  text: 'Только по тексту',
  semantic: 'Только по смыслу',
};
const SORT_LABELS: Record<SortMode, string> = {
  newest: 'Сначала новые',
  oldest: 'Сначала старые',
  'most-liked': 'Больше лайков',
  'least-liked': 'Меньше лайков',
  'most-saved': 'Чаще в избранном',
  random: 'Случайный порядок',
};
const STYLE_PRESETS = [
  { id: 'custom', title: 'Свой стиль', subtitle: 'Ваш голос, точная структура', initials: 'ВЫ', color: '#f7cf83', description: 'Сохраняет вашу манеру речи, помогает выстроить сетап и панчлайн.' },
  { id: 'carlin', title: 'Джордж Карлин', subtitle: 'Острая социальная сатира', initials: 'К', color: '#b28cff', description: 'Абсурд правил и слов, точные наблюдения, нарастающий ритм.' },
  { id: 'zhvanetsky', title: 'Михаил Жванецкий', subtitle: 'Тонкая бытовая ирония', initials: 'Ж', color: '#61d8d6', description: 'Короткие афористичные фразы и спокойное недосказанное.' },
  { id: 'carter', title: 'Джуди Картер', subtitle: 'Личное превращается в панчлайн', initials: 'КТ', color: '#ff779f', description: 'Уязвимость, эмоция «что меня бесит или пугает» и акт-ауты.' },
] as const;
const ONBOARDING = [
  { icon: Search, eyebrow: '01 / ПОИСК', title: 'Найдите шутку в тему', text: 'Ищите по точным словам или опишите ситуацию своими словами. Голосовой ввод поможет, когда руки заняты, а карта галактики покажет, где живут похожие шутки.' },
  { icon: Sparkles, eyebrow: '02 / СТЕНДАП', title: 'Ваша история — уже материал', text: 'Расскажите личную историю, подберите к ней подходящие шутки и соберите цельный монолог в выбранной манере. На старте вас ждут 50 бесплатных генераций.' },
  { icon: Bookmark, eyebrow: '03 / КОЛЛЕКЦИЯ', title: 'Собирайте своё смешное', text: 'Добавляйте шутки: ИИ предложит смысловые теги и подходящий кластер. Ставьте лайки, сохраняйте избранное и делитесь страницей автора.' },
  { icon: Compass, eyebrow: '04 / НА ТЕЛЕФОН', title: 'Галактика всегда под рукой', text: 'iPhone: откройте сайт в Safari, нажмите «Поделиться» → «На экран Домой». Android: в Chrome выберите «Установить приложение» или подтвердите предложение об установке.' },
];

function initials(name: string) {
  return name.split(/[\s-]+/).slice(0, 2).map((part) => part[0]).join('').toLocaleUpperCase('ru');
}

function Avatar({ user, size = 'normal' }: { user: Author; size?: 'small' | 'normal' | 'large' }) {
  const colors = ['#f4c46d', '#63d7dd', '#a990ff', '#fb779d', '#6ca9ff'];
  const color = colors[(user.username.length + user.id.length) % colors.length];
  return <span className={`avatar avatar-${size}`} style={{ '--avatar-color': color } as React.CSSProperties} aria-label={user.username}>
    <span>{initials(user.username)}</span>
  </span>;
}

function Toast({ message, onClose }: { message: string; onClose: () => void }) {
  useEffect(() => {
    const timer = window.setTimeout(onClose, 3600);
    return () => window.clearTimeout(timer);
  }, [message, onClose]);
  return <div className="toast" role="status"><span className="toast-mark"><Check size={15} /></span>{message}<button onClick={onClose} aria-label="Закрыть"><X size={15} /></button></div>;
}

function JokeCard({
  joke, compact = false, onLike, onFavorite, onTag, onProfile, onOpen, onShare,
}: {
  joke: Joke; compact?: boolean; onLike: (id: string) => void; onFavorite: (id: string) => void;
  onTag: (tag: string) => void; onProfile: (id: string) => void; onOpen: (id: string) => void; onShare: (id: string) => void;
}) {
  return <article className={`joke-card ${compact ? 'joke-card-compact' : ''}`}>
    <div className="joke-card-top">
      <button className="author-line" onClick={() => onProfile(joke.author.id)}>
        <Avatar user={joke.author} size="small" />
        <span className="author-name">{joke.author.username}</span>
        <span className="author-dot">·</span>
        <span className="joke-time"><Clock3 size={11} />{new Intl.DateTimeFormat('ru-RU', { day: 'numeric', month: 'short' }).format(new Date(joke.createdAt))}</span>
      </button>
      <button className="cluster-badge" style={{ '--cluster-color': joke.cluster.color } as React.CSSProperties} onClick={() => onTag(`cluster:${joke.cluster.slug}`)}>
        <span className="cluster-dot" />{joke.cluster.name}
      </button>
    </div>
    <button className="joke-text-button" onClick={() => onOpen(joke.id)} aria-label="Открыть шутку">
      <p className="joke-copy">{joke.text}</p>
    </button>
    <div className="joke-tags" aria-label="Теги шутки">
      {joke.triggers.slice(0, compact ? 3 : 5).map((tag) => <button key={tag} className="tag-pill" onClick={() => onTag(tag)}>#{tag}</button>)}
      {joke.triggers.length > (compact ? 3 : 5) && <span className="tag-extra">+{joke.triggers.length - (compact ? 3 : 5)}</span>}
    </div>
    <div className="joke-card-bottom">
      <div className="joke-actions">
        <button className={`icon-action like-action ${joke.liked ? 'is-active' : ''}`} onClick={() => onLike(joke.id)} aria-label={joke.liked ? 'Убрать лайк' : 'Поставить лайк'}>
          <Heart size={16} fill={joke.liked ? 'currentColor' : 'none'} /><span>{joke.likesCount}</span>
        </button>
        <button className={`icon-action ${joke.favorited ? 'is-active save-action' : ''}`} onClick={() => onFavorite(joke.id)} aria-label={joke.favorited ? 'Убрать из избранного' : 'В избранное'}>
          <Bookmark size={15} fill={joke.favorited ? 'currentColor' : 'none'} /><span>{joke.favoritesCount}</span>
        </button>
      </div>
      <div className="card-right-actions"><button className="card-share" onClick={() => onShare(joke.id)} aria-label="Поделиться"><Share2 size={14} /></button><button className="text-link joke-detail-link" onClick={() => onOpen(joke.id)}>Открыть <ArrowUpRight size={14} /></button></div>
    </div>
  </article>;
}

function EmptyState({ title, text, onClick, button }: { title: string; text: string; onClick?: () => void; button?: string }) {
  return <div className="empty-state"><span className="empty-icon"><Sparkles size={20} /></span><h3>{title}</h3><p>{text}</p>{button && onClick && <button className="button button-secondary button-small" onClick={onClick}>{button}<ArrowRight size={14} /></button>}</div>;
}

function App() {
  const [page, setPage] = useState<'home' | 'standup' | 'favorites' | 'profile' | 'detail' | 'admin'>('home');
  const [profileId, setProfileId] = useState('me');
  const [detailId, setDetailId] = useState('');
  const [allJokes, setAllJokes] = useState<Joke[]>([]);
  const [selectedJoke, setSelectedJoke] = useState<Joke | null>(null);
  const [popularTriggers, setPopularTriggers] = useState<string[]>([]);
  const [hoveredJoke, setHoveredJoke] = useState<Joke | null>(null);
  const [visibleCount, setVisibleCount] = useState(8);
  const [serverPage, setServerPage] = useState(1);
  const [serverHasMore, setServerHasMore] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [searchText, setSearchText] = useState('');
  const [submittedQuery, setSubmittedQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('hybrid');
  const [searchResponse, setSearchResponse] = useState<JokeSearchResponse | null>(null);
  const [isSearching, setIsSearching] = useState(false);
  const [sortMode, setSortMode] = useState<SortMode>('newest');
  const [clusterFilters, setClusterFilters] = useState<string[]>([]);
  const [activeTrigger, setActiveTrigger] = useState('');
  const [showClusterMenu, setShowClusterMenu] = useState(false);
  const [onlyMine, setOnlyMine] = useState(false);
  const [showOnboarding, setShowOnboarding] = useState(false);
  const [onboardingStep, setOnboardingStep] = useState(0);
  const [toastMessage, setToastMessage] = useState('');
  const [showAddModal, setShowAddModal] = useState(false);
  const [showCreditsModal, setShowCreditsModal] = useState(false);
  const [showMobileMenu, setShowMobileMenu] = useState(false);
  const [installPrompt, setInstallPrompt] = useState<any>(null);
  const [currentUser, setCurrentUser] = useState<Author | null>(null);
  const [publicProfile, setPublicProfile] = useState<Author | null>(null);
  const [backendAvailable, setBackendAvailable] = useState(false);
  const [credits, setCredits] = useState(0);
  const sentinelRef = useRef<HTMLDivElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);

  const showToast = useCallback((message: string) => setToastMessage(message), []);
  const refreshPopularTriggers = useCallback(async () => {
    try {
      const response = await fetch('/api/triggers');
      if (!response.ok) throw new Error('Не удалось получить теги.');
      const payload = await response.json();
      setPopularTriggers(Array.isArray(payload.triggers) ? payload.triggers.map((item: { tag?: string }) => item.tag).filter((tag: unknown): tag is string => typeof tag === 'string' && tag.length > 0) : []);
    } catch { setPopularTriggers([]); }
  }, []);

  const readLocation = useCallback(() => {
    const path = window.location.pathname;
    const parts = path.split('/').filter(Boolean).map(decodeURIComponent);
    if (parts[0] === 'joke' && parts[1]) { setPage('detail'); setDetailId(parts[1]); return; }
    if (parts[0] === 'user' && parts[1]) { setPage('profile'); setProfileId(parts[1]); return; }
    if (parts[0] === 'admin') { setPage('admin'); return; }
    if (parts[0] === 'standup') { setPage('standup'); return; }
    if (parts[0] === 'favorites') { setPage('favorites'); return; }
    if (parts[0] === 'cluster' && parts[1]) { setPage('home'); setClusterFilters([parts[1]]); setActiveTrigger(''); return; }
    if (parts[0] === 'tag' && parts[1]) { setPage('home'); setActiveTrigger(parts[1]); setClusterFilters([]); return; }
    setPage('home');
  }, []);

  const navigate = useCallback((path: string) => {
    window.history.pushState({}, '', path);
    readLocation();
    setShowMobileMenu(false);
    window.scrollTo({ top: 0, behavior: 'smooth' });
  }, [readLocation]);

  useEffect(() => {
    readLocation();
    const handlePop = () => readLocation();
    window.addEventListener('popstate', handlePop);
    localStorage.removeItem('jg-credits');
    void refreshPopularTriggers();
    const seen = localStorage.getItem('jg-onboarding-complete');
    if (!seen) setShowOnboarding(true);
    const handleInstall = (event: Event) => { event.preventDefault(); setInstallPrompt(event); };
    window.addEventListener('beforeinstallprompt', handleInstall);
    const initialParams = new URLSearchParams({ page: '1', limit: '50' });
    const initialParts = window.location.pathname.split('/').filter(Boolean).map(decodeURIComponent);
    if (initialParts[0] === 'cluster' && initialParts[1]) initialParams.set('cluster', initialParts[1]);
    if (initialParts[0] === 'tag' && initialParts[1]) initialParams.set('tag', initialParts[1]);
    if (initialParts[0] === 'user' && initialParts[1] && initialParts[1] !== 'me') initialParams.set('author', initialParts[1]);
    fetch(`/api/jokes?${initialParams}`)
      .then((response) => response.ok ? response.json() : null)
      .then((payload) => {
        if (payload && Array.isArray(payload.jokes)) {
          setAllJokes((current) => {
            const merged = new Map(current.map((item) => [item.id, item]));
            payload.jokes.forEach((item: Joke) => merged.set(item.id, item));
            return [...merged.values()];
          });
          setSelectedJoke(payload.jokes[0] ?? null);
          setServerPage(payload.page ?? 1);
          setServerHasMore(Boolean(payload.has_more));
        }
      }).catch(() => undefined);
    fetch('/api/me').then((response) => {
      if (!response.ok) throw new Error('Сервер приложения недоступен.');
      setBackendAvailable(true);
      return response.json();
    }).then((payload) => {
      if (payload?.authenticated && payload.user) {
        const rawUser = payload.user;
        const account: Author = {
          id: String(rawUser.id),
          username: rawUser.username || 'Пользователь',
          avatarUrl: rawUser.avatar_url || rawUser.avatarUrl,
          email: rawUser.email || '',
          bio: rawUser.bio || '',
          createdAt: rawUser.created_at,
        };
        setCurrentUser(account);
        setCredits(Number.isFinite(Number(rawUser.standup_credits)) ? Number(rawUser.standup_credits) : 0);
        if (rawUser.onboarding_completed) {
          localStorage.setItem('jg-onboarding-complete', 'true');
          setShowOnboarding(false);
        } else {
          localStorage.removeItem('jg-onboarding-complete');
          setOnboardingStep(0);
          setShowOnboarding(true);
        }
      } else {
        setCurrentUser(null);
        setCredits(0);
      }
    }).catch(() => { setBackendAvailable(false); setCurrentUser(null); setCredits(0); });
    return () => {
      window.removeEventListener('popstate', handlePop);
      window.removeEventListener('beforeinstallprompt', handleInstall);
    };
  }, [readLocation, refreshPopularTriggers]);

  useEffect(() => {
    if (page !== 'profile') { setPublicProfile(null); return; }
    const targetID = profileId === 'me' ? currentUser?.id : profileId;
    const isMyProfile = Boolean(currentUser && targetID === currentUser.id);
    if (!targetID || (!backendAvailable && !isMyProfile)) { setPublicProfile(null); return; }
    const controller = new AbortController();
    if (!isMyProfile) setPublicProfile(null);
    const loadProfile = async () => {
      try {
        if (!isMyProfile) {
          const response = await fetch(`/api/users/${encodeURIComponent(targetID)}`, { signal: controller.signal });
          if (!response.ok) return;
          const payload = await response.json();
          const rawUser = payload.user ?? payload;
          setPublicProfile({
            id: String(rawUser.id), username: rawUser.username || 'Пользователь',
            avatarUrl: rawUser.avatar_url || rawUser.avatarUrl, bio: rawUser.bio || '',
            createdAt: rawUser.created_at, jokesCount: Number(rawUser.jokes_count ?? 0), totalLikes: Number(rawUser.likes_count ?? 0),
          });
        }
        if (!backendAvailable) return;
        const jokesResponse = await fetch(`/api/jokes?author=${encodeURIComponent(targetID)}&page=1&limit=50`, { signal: controller.signal });
        if (!jokesResponse.ok) return;
        const jokesPayload = await jokesResponse.json();
        const batch: Joke[] = Array.isArray(jokesPayload.jokes) ? jokesPayload.jokes : [];
        setAllJokes((current) => {
          const merged = new Map(current.map((item) => [item.id, item]));
          batch.forEach((item) => merged.set(item.id, item));
          return [...merged.values()];
        });
        setServerPage(jokesPayload.page ?? 1);
        setServerHasMore(Boolean(jokesPayload.has_more));
      } catch { /* A cached profile or the server-rendered card remains available offline. */ }
    };
    void loadProfile();
    return () => controller.abort();
  }, [page, profileId, currentUser?.id, backendAvailable]);

  useEffect(() => {
    if (!backendAvailable || page !== 'home') return;
    const params = new URLSearchParams({ page: '1', limit: '50' });
    let hasFilter = false;
    if (clusterFilters.length === 1) { params.set('cluster', clusterFilters[0]); hasFilter = true; }
    if (activeTrigger) { params.set('tag', activeTrigger); hasFilter = true; }
    if (onlyMine && currentUser) { params.set('author', currentUser.id); hasFilter = true; }
    if (!hasFilter) return;
    const controller = new AbortController();
    fetch(`/api/jokes?${params}`, { signal: controller.signal }).then((response) => response.ok ? response.json() : null).then((payload) => {
      if (!payload || !Array.isArray(payload.jokes)) return;
      setAllJokes((current) => {
        const merged = new Map(current.map((item) => [item.id, item]));
        payload.jokes.forEach((item: Joke) => merged.set(item.id, item));
        return [...merged.values()];
      });
      setServerPage(payload.page ?? 1);
      setServerHasMore(Boolean(payload.has_more));
    }).catch(() => undefined);
    return () => controller.abort();
  }, [backendAvailable, page, clusterFilters, activeTrigger, onlyMine, currentUser?.id]);

  useEffect(() => {
    if (page !== 'favorites' || !backendAvailable) return;
    if (!currentUser) { setServerHasMore(false); return; }
    const controller = new AbortController();
    fetch('/api/jokes?page=1&limit=50&favorites=true', { signal: controller.signal }).then((response) => response.ok ? response.json() : null).then((payload) => {
      if (!payload || !Array.isArray(payload.jokes)) return;
      setAllJokes((current) => {
        const merged = new Map(current.map((item) => [item.id, item]));
        payload.jokes.forEach((item: Joke) => merged.set(item.id, item));
        return [...merged.values()];
      });
      setServerPage(payload.page ?? 1);
      setServerHasMore(Boolean(payload.has_more));
    }).catch(() => undefined);
    return () => controller.abort();
  }, [page, backendAvailable, currentUser?.id]);

  const loadMoreServer = useCallback(async () => {
    if (!serverHasMore || loadingMore) return;
    setLoadingMore(true);
    const nextPage = serverPage + 1;
    const params = new URLSearchParams({ page: String(nextPage), limit: '50' });
    if (page === 'home') {
      if (clusterFilters.length === 1) params.set('cluster', clusterFilters[0]);
      if (activeTrigger) params.set('tag', activeTrigger);
      if (onlyMine && currentUser) params.set('author', currentUser.id);
    } else if (page === 'favorites') {
      params.set('favorites', 'true');
    } else if (page === 'profile') {
      const authorID = profileId === 'me' ? currentUser?.id : profileId;
      if (authorID) params.set('author', authorID);
    }
    try {
      const response = await fetch(`/api/jokes?${params}`);
      if (!response.ok) throw new Error('Подгрузка недоступна');
      const payload = await response.json();
      const batch: Joke[] = Array.isArray(payload.jokes) ? payload.jokes : [];
      setAllJokes((current) => {
        const known = new Set(current.map((joke) => joke.id));
        return [...current, ...batch.filter((joke) => !known.has(joke.id))];
      });
      setServerPage(nextPage);
      setServerHasMore(Boolean(payload.has_more));
    } catch { setServerHasMore(false); }
    finally { setLoadingMore(false); }
  }, [serverHasMore, loadingMore, serverPage, page, clusterFilters, activeTrigger, onlyMine, currentUser, profileId]);

  const finishOnboarding = () => {
    localStorage.setItem('jg-onboarding-complete', 'true');
    setShowOnboarding(false);
    fetch('/api/onboarding/complete', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' }).catch(() => undefined);
  };

  const startVoice = (setter: (value: string) => void, current = '') => {
    const speechWindow = window as Window & { SpeechRecognition?: new () => any; webkitSpeechRecognition?: new () => any };
    const Speech = speechWindow.SpeechRecognition || speechWindow.webkitSpeechRecognition;
    if (!Speech) { showToast('Голосовой ввод не поддерживается этим браузером. Можно ввести текст вручную.'); return; }
    const recognition = new Speech();
    recognition.lang = 'ru-RU';
    recognition.interimResults = false;
    recognition.maxAlternatives = 1;
    recognition.onresult = (event: any) => {
      const transcript = event.results?.[0]?.[0]?.transcript ?? '';
      setter(current ? `${current} ${transcript}` : transcript);
    };
    recognition.onerror = () => showToast('Не удалось распознать речь. Проверьте доступ к микрофону.');
    recognition.start();
    showToast('Слушаю… Говорите по-русски.');
  };

  const runSearch = async (query = searchText, mode = searchMode) => {
    const trimmed = query.trim();
    if (!trimmed) { setSubmittedQuery(''); setSearchResponse(null); return; }
    setSubmittedQuery(trimmed);
    setIsSearching(true);
    try {
      let result: JokeSearchResponse;
      try {
        result = await searchJokes(trimmed, mode, allJokes);
        if (!result.keyword_results?.length && !result.semantic_results?.length) {
          const local = rankLocally(trimmed, allJokes, mode);
          result = { keyword_results: local.keyword, semantic_results: local.semantic, query: trimmed, mode };
        }
      } catch {
        const local = rankLocally(trimmed, allJokes, mode);
        result = { keyword_results: local.keyword, semantic_results: local.semantic, query: trimmed, mode };
      }
      setSearchResponse(result);
      const matches = [...(result.keyword_results ?? []), ...(result.semantic_results ?? [])];
      if (matches.length) {
        setAllJokes((current) => {
          const merged = new Map(current.map((item) => [item.id, item]));
          matches.forEach((item) => merged.set(item.id, item));
          return [...merged.values()];
        });
      }
      const first = matches[0];
      if (first) setSelectedJoke(first);
      if (!first) showToast('Пока не нашли совпадений. Попробуйте переформулировать запрос.');
    } finally { setIsSearching(false); }
  };

  const allSearchResultIds = useMemo(() => new Set([
    ...(searchResponse?.keyword_results ?? []).map((joke) => joke.id),
    ...(searchResponse?.semantic_results ?? []).map((joke) => joke.id),
  ]), [searchResponse]);

  const filteredJokes = useMemo(() => {
    let list = [...allJokes];
    if (clusterFilters.length) list = list.filter((joke) => clusterFilters.includes(joke.cluster.slug));
    if (activeTrigger) list = list.filter((joke) => joke.triggers.some((tag) => tag.toLocaleLowerCase('ru') === activeTrigger.toLocaleLowerCase('ru')));
    if (onlyMine) list = currentUser ? list.filter((joke) => joke.author.id === currentUser.id) : [];
    if (page === 'favorites') list = list.filter((joke) => joke.favorited);
    if (page === 'profile' && profileId !== 'me') list = list.filter((joke) => joke.author.id === profileId);
    switch (sortMode) {
      case 'oldest': list.sort((a, b) => new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime()); break;
      case 'most-liked': list.sort((a, b) => b.likesCount - a.likesCount); break;
      case 'least-liked': list.sort((a, b) => a.likesCount - b.likesCount); break;
      case 'most-saved': list.sort((a, b) => b.favoritesCount - a.favoritesCount); break;
      case 'random': list.sort((a, b) => (Number(a.id) * 47 % 23) - (Number(b.id) * 47 % 23)); break;
      default: list.sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime());
    }
    return list;
  }, [allJokes, clusterFilters, activeTrigger, onlyMine, page, profileId, sortMode, currentUser]);

  useEffect(() => { setVisibleCount(8); }, [page, clusterFilters, activeTrigger, onlyMine, sortMode, submittedQuery]);
  useEffect(() => {
    if (page !== 'home' && page !== 'favorites' && page !== 'profile') return;
    const element = sentinelRef.current;
    if (!element) return;
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) setVisibleCount((count) => Math.min(count + 6, filteredJokes.length));
    }, { rootMargin: '240px' });
    observer.observe(element);
    return () => observer.disconnect();
  }, [page, filteredJokes.length]);

  const toggleLike = (id: string) => {
    if (backendAvailable && !currentUser) { showToast('Войдите через Яндекс ID, чтобы ставить лайки.'); return; }
    setAllJokes((current) => current.map((joke) => joke.id === id
      ? { ...joke, liked: !joke.liked, likesCount: joke.likesCount + (joke.liked ? -1 : 1) }
      : joke));
    if (currentUser) toggleJokeAction(id, 'like').catch(() => showToast('Не удалось сохранить лайк. Попробуйте ещё раз.'));
  };
  const toggleFavorite = (id: string) => {
    if (backendAvailable && !currentUser) { showToast('Войдите через Яндекс ID, чтобы сохранять шутки.'); return; }
    setAllJokes((current) => current.map((joke) => joke.id === id
      ? { ...joke, favorited: !joke.favorited, favoritesCount: joke.favoritesCount + (joke.favorited ? -1 : 1) }
      : joke));
    if (currentUser) toggleJokeAction(id, 'favorite').catch(() => showToast('Не удалось сохранить шутку. Попробуйте ещё раз.'));
    showToast('Сохранено в вашей коллекции.');
  };
  const openJoke = (id: string) => {
    const joke = allJokes.find((item) => item.id === id);
    if (joke) setSelectedJoke(joke);
    setDetailId(id);
    navigate(`/joke/${encodeURIComponent(id)}`);
  };
  const openProfile = (id: string) => navigate(`/user/${encodeURIComponent(id)}`);
  const openTag = (value: string) => {
    if (value.startsWith('cluster:')) navigate(`/cluster/${value.slice('cluster:'.length)}`);
    else navigate(`/tag/${encodeURIComponent(value)}`);
  };
  const shareJoke = async (id: string) => {
    const url = `${window.location.origin}/joke/${encodeURIComponent(id)}`;
    try {
      if (navigator.share) await navigator.share({ title: 'Шутка из Галактики юмора', url });
      else { await navigator.clipboard.writeText(url); showToast('Ссылка скопирована.'); }
    } catch { showToast('Не получилось поделиться ссылкой.'); }
  };

  const closeAddModal = () => setShowAddModal(false);
  const handleInstall = async () => {
    if (installPrompt) { await installPrompt.prompt(); setInstallPrompt(null); return; }
    setOnboardingStep(3); setShowOnboarding(true);
  };

  const signOut = async () => {
    if (backendAvailable) {
      try {
        const response = await fetch('/auth/logout', { method: 'POST' });
        if (!response.ok) throw new Error('Сервер не подтвердил выход.');
      } catch { showToast('Не удалось связаться с сервером. Попробуйте выйти ещё раз.'); return; }
    }
    setCurrentUser(null);
    setProfileId('me');
    navigate('/user/me');
    setCredits(0);
    localStorage.removeItem('jg-credits');
    showToast('Вы вышли из аккаунта.');
  };

  const currentProfile = useMemo<Author | null>(() => {
    if (profileId === 'me' || currentUser?.id === profileId) return currentUser;
    if (publicProfile?.id === profileId) return publicProfile;
    return allJokes.find((joke) => joke.author.id === profileId)?.author ?? null;
  }, [profileId, allJokes, currentUser, publicProfile]);
  const detailJoke = allJokes.find((joke) => joke.id === detailId) ?? null;
  const detailJokeLoaded = detailJoke !== null;
  useEffect(() => {
    if (page !== 'detail' || !detailId || !backendAvailable || detailJokeLoaded) return;
    const controller = new AbortController();
    fetch(`/api/jokes/${encodeURIComponent(detailId)}`, { signal: controller.signal })
      .then((response) => response.ok ? response.json() : null)
      .then((joke: Joke | null) => {
        if (!joke?.id) return;
        setAllJokes((current) => current.some((item) => item.id === joke.id) ? current : [...current, joke]);
      })
      .catch(() => undefined);
    return () => controller.abort();
  }, [page, detailId, backendAvailable, detailJokeLoaded]);
  useEffect(() => {
    if (selectedJoke && !allJokes.some((joke) => joke.id === selectedJoke.id)) {
      setSelectedJoke(allJokes[0] ?? null);
    }
  }, [allJokes, selectedJoke]);
  const similarJokes = detailJoke ? allJokes.filter((item) => item.id !== detailJoke.id)
    .map((item) => ({ item, score: item.triggers.filter((tag) => detailJoke.triggers.includes(tag)).length + (item.cluster.slug === detailJoke.cluster.slug ? 1 : 0) }))
    .sort((a, b) => b.score - a.score).slice(0, 3).map(({ item }) => item) : [];

  const navItems = [
    { id: 'home', label: 'Галактика', icon: Home, path: '/' },
    { id: 'standup', label: 'Студия стендапа', icon: Sparkles, path: '/standup' },
    { id: 'favorites', label: 'Избранное', icon: Bookmark, path: '/favorites' },
    { id: 'profile', label: 'Мой профиль', icon: UserRound, path: '/user/me' },
  ];

  return <View style={{ minHeight: '100vh' } as any}>
    <div className="app-frame">
      <aside className={`sidebar ${showMobileMenu ? 'sidebar-open' : ''}`}>
        <button className="brand-lockup" onClick={() => navigate('/')} aria-label="На главную">
          <span className="brand-icon"><span /><i /><b /></span>
          <span className="brand-copy"><strong>jokegalaxy</strong><small>юмор на своей орбите</small></span>
        </button>
        <div className="sidebar-label">РАЗДЕЛЫ</div>
        <nav className="primary-nav" aria-label="Основная навигация">
          {navItems.map((item) => {
            const Icon = item.icon;
            const active = (item.id === 'home' && page === 'home') || page === item.id;
            return <button className={`nav-item ${active ? 'nav-item-active' : ''}`} key={item.id} onClick={() => navigate(item.path)}>
              <Icon size={18} strokeWidth={1.8} /><span>{item.label}</span>{item.id === 'standup' && <span className="nav-spark">{credits}</span>}
            </button>;
          })}
        </nav>
        <div className="sidebar-separator" />
        <div className="sidebar-label sidebar-label-row"><span>ВАШИ КЛАСТЕРЫ</span><button onClick={() => setShowClusterMenu((value) => !value)} aria-label="Настроить кластеры"><Tags size={14} /></button></div>
        <div className="sidebar-clusters">
          {clusters.slice(0, 5).map((cluster) => <button key={cluster.slug} className={`sidebar-cluster ${clusterFilters.includes(cluster.slug) ? 'sidebar-cluster-active' : ''}`} onClick={() => {
            setClusterFilters((current) => current.includes(cluster.slug) ? current.filter((slug) => slug !== cluster.slug) : [...current, cluster.slug]);
            setPage('home');
          }}><span className="cluster-dot" style={{ background: cluster.color }} />{cluster.name}</button>)}
          <button className="sidebar-more" onClick={() => setShowClusterMenu((value) => !value)}>Все кластеры <ChevronDown size={13} /></button>
        </div>
        <div className="sidebar-bottom">
          {currentUser && <div className="credit-mini"><span className="credit-icon"><Zap size={15} fill="currentColor" /></span><div><strong>{credits} генераций</strong><small>на вашем балансе</small></div><button onClick={() => setShowCreditsModal(true)} aria-label="Пополнить баланс"><Plus size={15} /></button></div>}
          <button className="install-cta" onClick={handleInstall}><span className="install-star"><Sparkles size={14} /></span><span><b>С собой в телефоне</b><small>Установить приложение</small></span><ArrowUpRight size={15} /></button>
          {currentUser ? <button className="profile-mini" onClick={() => navigate('/user/me')}><Avatar user={currentUser} size="small" /><span><b>{currentUser.username}</b><small>Ваш профиль</small></span><ChevronRight size={14} /></button> : <button className="profile-mini" onClick={() => backendAvailable ? window.location.assign('/auth/yandex/login') : showToast('Вход станет доступен после подключения сервера.')}><span className="avatar avatar-small account-action-icon"><UserRound size={15} /></span><span><b>{backendAvailable ? 'Войти через Яндекс' : 'Нет подключения к серверу'}</b><small>Профиль появится после входа</small></span><ChevronRight size={14} /></button>}
          <a className="admin-subtle-link" href="/admin" onClick={(event) => { event.preventDefault(); navigate('/admin'); }}><Shield size={13} /> Панель редакции</a>
        </div>
      </aside>

      <main className="main-area">
        <header className="topbar">
          <button className="mobile-menu-button" onClick={() => setShowMobileMenu((value) => !value)} aria-label="Открыть меню">{showMobileMenu ? <X size={20} /> : <Menu size={20} />}</button>
          <div className="topbar-breadcrumb"><span>ГАЛАКТИКА ЮМОРА</span><ChevronRight size={13} /><span className="breadcrumb-current">{page === 'home' ? 'Обзор' : page === 'standup' ? 'Студия стендапа' : page === 'favorites' ? 'Избранное' : page === 'profile' ? 'Профиль автора' : page === 'detail' ? 'Шутка' : 'Администрирование'}</span></div>
          <div className="topbar-actions">{currentUser && <button className="balance-pill" onClick={() => setShowCreditsModal(true)}><Zap size={14} fill="currentColor" />{credits}<span>генераций</span></button>}<button className="button button-primary button-top-add" onClick={() => setShowAddModal(true)}><Plus size={16} />Добавить шутку</button>{currentUser ? <button className="top-avatar" onClick={() => navigate('/user/me')} aria-label="Мой профиль"><Avatar user={currentUser} size="small" /></button> : <button className="button button-secondary button-login" disabled={!backendAvailable} onClick={() => window.location.assign('/auth/yandex/login')}>Войти</button>}</div>
        </header>

        <div className="content-wrap">
          {page === 'home' && <HomePage
            allJokes={allJokes} filteredJokes={filteredJokes} popularTriggers={popularTriggers} visibleCount={visibleCount} setVisibleCount={setVisibleCount}
            selectedJoke={selectedJoke} setSelectedJoke={setSelectedJoke} hoveredJoke={hoveredJoke} setHoveredJoke={setHoveredJoke}
            searchText={searchText} setSearchText={setSearchText} searchInputRef={searchInputRef} submittedQuery={submittedQuery}
            searchMode={searchMode} setSearchMode={(mode) => { setSearchMode(mode); if (submittedQuery) void runSearch(submittedQuery, mode); }}
            searchResponse={searchResponse} allSearchResultIds={allSearchResultIds} isSearching={isSearching} runSearch={runSearch}
            activeTrigger={activeTrigger} clusterFilters={clusterFilters} setClusterFilters={setClusterFilters}
            sortMode={sortMode} setSortMode={setSortMode} onlyMine={onlyMine} setOnlyMine={setOnlyMine}
            showClusterMenu={showClusterMenu} setShowClusterMenu={setShowClusterMenu} sentinelRef={sentinelRef}
            serverHasMore={serverHasMore} loadingMore={loadingMore} onLoadMore={loadMoreServer}
            onLike={toggleLike} onFavorite={toggleFavorite} onTag={openTag} onProfile={openProfile} onOpen={openJoke} onShare={shareJoke}
            onVoice={() => startVoice((value) => { setSearchText(value); searchInputRef.current?.focus(); })}
            onAdd={() => setShowAddModal(true)} onClear={() => { setSubmittedQuery(''); setSearchResponse(null); setSearchText(''); setActiveTrigger(''); setClusterFilters([]); }}
          />}
          {page === 'standup' && <StandupPage credits={credits} setCredits={setCredits} jokes={allJokes} showToast={showToast} backendAvailable={backendAvailable} authenticated={Boolean(currentUser)} onTopUp={() => setShowCreditsModal(true)} onVoice={(setter, current) => startVoice(setter, current)} />}
          {page === 'favorites' && <FavoritesPage jokes={filteredJokes} onLike={toggleLike} onFavorite={toggleFavorite} onTag={openTag} onProfile={openProfile} onOpen={openJoke} onShare={shareJoke} onExplore={() => navigate('/')} authRequired={backendAvailable && !currentUser} hasMore={serverHasMore} loadingMore={loadingMore} onLoadMore={loadMoreServer} />}
          {page === 'profile' && (currentProfile ? <ProfilePage user={currentProfile} isOwner={profileId === 'me' || profileId === currentUser?.id} authenticated={Boolean(currentUser)} onLogout={signOut} jokes={filteredJokes} allJokes={allJokes} onLike={toggleLike} onFavorite={toggleFavorite} onTag={openTag} onProfile={openProfile} onOpen={openJoke} onShare={shareJoke} onAdd={() => setShowAddModal(true)} hasMore={serverHasMore} loadingMore={loadingMore} onLoadMore={loadMoreServer} /> : <ProfileUnavailable profileId={profileId} backendAvailable={backendAvailable} />)}
          {page === 'detail' && detailJoke ? <DetailPage joke={detailJoke} similar={similarJokes} onLike={toggleLike} onFavorite={toggleFavorite} onTag={openTag} onProfile={openProfile} onOpen={openJoke} onShare={shareJoke} onBack={() => navigate('/')} /> : page === 'detail' ? <section className="detail-page"><EmptyState title="Шутка не найдена" text="В коллекции пока нет этой публикации." button="Вернуться в галактику" onClick={() => navigate('/')} /></section> : null}
          {page === 'admin' && <AdminConsole jokes={allJokes} setJokes={setAllJokes} onlineMode={backendAvailable} showToast={showToast} />}
        </div>
        <footer className="app-footer"><span>© 2026 Галактика юмора</span><span>Собрано с <span className="footer-heart">♥</span> и самоиронией</span><button onClick={() => { setOnboardingStep(0); setShowOnboarding(true); }}>Как это работает?</button></footer>
      </main>
      {showMobileMenu && <button className="mobile-scrim" onClick={() => setShowMobileMenu(false)} aria-label="Закрыть меню" />}
    </div>

    {showOnboarding && <OnboardingModal step={onboardingStep} setStep={setOnboardingStep} onClose={finishOnboarding} />}
    {showAddModal && <AddJokeModal onClose={closeAddModal} onlineMode={backendAvailable} authRequired={!backendAvailable || !currentUser} onSave={(joke) => {
      setAllJokes((current) => [joke, ...current]); setSelectedJoke(joke); setVisibleCount((count) => count + 1); void refreshPopularTriggers(); showToast('Шутка добавлена в Галактику. Спасибо!'); closeAddModal();
    }} onToast={showToast} />}
    {showCreditsModal && <CreditsModal onClose={() => setShowCreditsModal(false)} credits={credits} backendAvailable={backendAvailable} authenticated={Boolean(currentUser)} email={currentUser?.email ?? ''} onToast={showToast} />}
    {toastMessage && <Toast message={toastMessage} onClose={() => setToastMessage('')} />}
  </View>;
}

function HomePage(props: {
  allJokes: Joke[]; filteredJokes: Joke[]; popularTriggers: string[]; visibleCount: number; setVisibleCount: React.Dispatch<React.SetStateAction<number>>;
  selectedJoke: Joke | null; setSelectedJoke: (joke: Joke | null) => void; hoveredJoke: Joke | null; setHoveredJoke: (joke: Joke | null) => void;
  searchText: string; setSearchText: (value: string) => void; searchInputRef: React.RefObject<HTMLInputElement>; submittedQuery: string;
  searchMode: SearchMode; setSearchMode: (mode: SearchMode) => void; searchResponse: JokeSearchResponse | null; allSearchResultIds: Set<string>;
  isSearching: boolean; runSearch: (query?: string, mode?: SearchMode) => Promise<void>; activeTrigger: string; clusterFilters: string[];
  setClusterFilters: React.Dispatch<React.SetStateAction<string[]>>; sortMode: SortMode; setSortMode: (mode: SortMode) => void;
  onlyMine: boolean; setOnlyMine: (value: boolean) => void; showClusterMenu: boolean; setShowClusterMenu: (value: boolean | ((old: boolean) => boolean)) => void;
  sentinelRef: React.RefObject<HTMLDivElement>; serverHasMore: boolean; loadingMore: boolean; onLoadMore: () => Promise<void>;
  onLike: (id: string) => void; onFavorite: (id: string) => void; onTag: (tag: string) => void;
  onProfile: (id: string) => void; onOpen: (id: string) => void; onShare: (id: string) => void; onVoice: () => void; onAdd: () => void;
  onClear: () => void;
}) {
  const {
    allJokes, filteredJokes, popularTriggers, visibleCount, setVisibleCount, selectedJoke, setSelectedJoke, hoveredJoke, setHoveredJoke,
    searchText, setSearchText, searchInputRef, submittedQuery, searchMode, setSearchMode, searchResponse, allSearchResultIds,
    isSearching, runSearch, activeTrigger, clusterFilters, setClusterFilters, sortMode, setSortMode, onlyMine, setOnlyMine,
    showClusterMenu, setShowClusterMenu, sentinelRef, serverHasMore, loadingMore, onLoadMore,
    onLike, onFavorite, onTag, onProfile, onOpen, onShare, onVoice, onAdd, onClear,
  } = props;
  const galaxyJokes = useMemo(() => allJokes.filter((joke) => !clusterFilters.length || clusterFilters.includes(joke.cluster.slug)), [allJokes, clusterFilters]);
  const displayJokes = filteredJokes.slice(0, visibleCount);
  const keywordResults = searchResponse?.keyword_results ?? [];
  const semanticResults = searchResponse?.semantic_results ?? [];
  const activeClusterNames = clusters.filter((cluster) => clusterFilters.includes(cluster.slug)).map((cluster) => cluster.name);

  useEffect(() => {
    if (visibleCount >= filteredJokes.length && serverHasMore && !loadingMore) void onLoadMore();
  }, [visibleCount, filteredJokes.length, serverHasMore, loadingMore, onLoadMore]);

  return <>
    <section className="page-intro home-intro">
      <div className="eyebrow"><span className="eyebrow-line" /> ИНТЕРАКТИВНАЯ КОЛЛЕКЦИЯ <span className="eyebrow-spark">✦</span></div>
      <div className="home-title-row"><div><h1>Найдите свою <span>шутку.</span></h1><p>Ищите не только по словам — ищите по ощущению.</p></div><button className="daily-note" onClick={() => allJokes.length ? setSelectedJoke(allJokes[Math.floor(Math.random() * allJokes.length)]) : onAdd()}><span className="daily-note-icon"><Sparkles size={16} /></span><span><b>{allJokes.length ? 'Есть минутка?' : 'Коллекция пуста'}</b><small>{allJokes.length ? 'Выбрать случайную шутку' : 'Опубликовать первую'}</small></span><ArrowUpRight size={15} /></button></div>
    </section>

    <section className="search-section" aria-label="Поиск шуток">
      <form className="search-box" onSubmit={(event) => { event.preventDefault(); void runSearch(); }}>
        <span className="search-leading"><Search size={19} /></span>
        <input ref={searchInputRef} value={searchText} onChange={(event) => setSearchText(event.target.value)} placeholder="Например: «когда дедлайн уже вчера»" aria-label="Найти шутку" />
        {searchText && <button type="button" className="clear-search" onClick={() => setSearchText('')} aria-label="Очистить поиск"><X size={16} /></button>}
        <button type="button" className="voice-button" onClick={onVoice} aria-label="Голосовой поиск"><Mic size={17} /><span>Голосом</span></button>
        <button className="search-submit" aria-label="Искать">{isSearching ? <LoaderCircle size={18} className="spin" /> : <ArrowRight size={18} />}</button>
      </form>
      <div className="search-control-row">
        <div className="mode-switch" role="tablist" aria-label="Режим поиска">
          {(['hybrid', 'text', 'semantic'] as SearchMode[]).map((mode) => <button key={mode} className={`mode-tab ${searchMode === mode ? 'mode-tab-active' : ''}`} role="tab" aria-selected={searchMode === mode} onClick={() => setSearchMode(mode)}>
            {mode === 'hybrid' && <Sparkles size={13} />}{mode === 'text' && <Search size={13} />}{mode === 'semantic' && <Compass size={13} />}{MODE_LABELS[mode]}
          </button>)}
        </div>
        <span className="search-hint">{searchMode === 'hybrid' ? 'Сначала совпадения, затем похожие по смыслу' : searchMode === 'text' ? 'Точные слова и триггеры' : 'Смысл и контекст, даже без общих слов'}</span>
      </div>
    </section>

    {submittedQuery && <section className="search-results-section" aria-live="polite">
      <div className="results-heading"><div><span className="section-kicker">РЕЗУЛЬТАТЫ ПОИСКА</span><h2>Для запроса <span className="query-highlight">«{submittedQuery}»</span></h2></div><button className="clear-results" onClick={onClear}><X size={14} /> Очистить поиск</button></div>
      <div className="results-meta"><span className="result-mode-pill"><Sparkles size={12} />{MODE_LABELS[searchMode]}</span><span>{(keywordResults.length + semanticResults.length).toLocaleString('ru-RU')} совпадений</span><span className="result-live-dot" />{isSearching ? 'Ищем в галактике…' : 'Релевантность учтена'}</div>
      {!keywordResults.length && !semanticResults.length && !isSearching && <div className="no-results-note"><Info size={15} />{allJokes.length ? 'Точных совпадений пока нет. Попробуйте изменить запрос.' : 'Коллекция пока пуста — добавьте первую шутку.'}</div>}
      {keywordResults.length > 0 && <div className="result-group"><div className="result-group-title"><span className="result-index">01</span><div><h3>Совпадения по словам</h3><p>Текст шутки и смысловые триггеры</p></div><span className="result-count">{keywordResults.length}</span></div><div className="result-card-grid">{keywordResults.slice(0, 4).map((joke) => <div key={joke.id} className="result-card-wrap"><div className="relevance-badge">{Math.round((joke.relevance ?? 0.82) * 100)}% в тему</div><JokeCard joke={joke} compact onLike={onLike} onFavorite={onFavorite} onTag={onTag} onProfile={onProfile} onOpen={onOpen} onShare={onShare} /></div>)}</div></div>}
      {searchMode === 'hybrid' && <div className="semantic-divider"><span /><div><Compass size={14} /> Похожие по смыслу</div><span /></div>}
      {semanticResults.length > 0 && <div className="result-group semantic-group"><div className="result-group-title"><span className="result-index semantic-index">{searchMode === 'semantic' ? '01' : '02'}</span><div><h3>{searchMode === 'semantic' ? 'По смыслу и ситуации' : 'Похожие по смыслу'}</h3><p>Нашли даже без общих слов</p></div><span className="result-count">{semanticResults.length}</span></div><div className="result-card-grid">{semanticResults.slice(0, 4).map((joke) => <div key={joke.id} className="result-card-wrap"><div className="relevance-badge semantic-relevance">{Math.round((joke.relevance ?? 0.76) * 100)}% близко</div><JokeCard joke={joke} compact onLike={onLike} onFavorite={onFavorite} onTag={onTag} onProfile={onProfile} onOpen={onOpen} onShare={onShare} /></div>)}</div></div>}
      {isSearching && <div className="search-loading"><LoaderCircle size={16} className="spin" /> Подбираем смысловые совпадения…</div>}
    </section>}

    {popularTriggers.length > 0 && <div className="popular-row"><div className="popular-label"><Tags size={14} /><span>ПОПУЛЯРНОЕ</span></div><div className="popular-tags">{popularTriggers.map((tag, index) => <button key={tag} className={`popular-tag ${activeTrigger === tag ? 'popular-tag-active' : ''}`} onClick={() => onTag(tag)}><span className={`popular-dot pop-dot-${index % 4}`} />{tag}</button>)}</div><button className="all-tags-link" onClick={() => setShowClusterMenu((value) => !value)}>Фильтры <ChevronDown size={13} /></button></div>}

    <div className="filter-panel">
      <div className="filter-left">
        <div className="cluster-filter-wrap">
          <button className={`filter-button ${clusterFilters.length ? 'filter-button-on' : ''}`} onClick={() => setShowClusterMenu((value) => !value)}><span className="filter-button-icon"><SlidersHorizontalIcon /></span>Темы{clusterFilters.length > 0 && <span className="filter-count">{clusterFilters.length}</span>}<ChevronDown size={14} /></button>
          {showClusterMenu && <div className="cluster-dropdown"><div className="dropdown-title"><b>Выберите темы</b><button onClick={() => setShowClusterMenu(false)} aria-label="Закрыть"><X size={14} /></button></div>{clusters.map((cluster) => <button key={cluster.slug} className="cluster-option" onClick={() => setClusterFilters((current) => current.includes(cluster.slug) ? current.filter((slug) => slug !== cluster.slug) : [...current, cluster.slug])}><span className="cluster-dot" style={{ background: cluster.color }} /><span>{cluster.name}</span>{clusterFilters.includes(cluster.slug) && <Check size={14} />}</button>)}<button className="dropdown-reset" onClick={() => setClusterFilters([])}>Сбросить выбор</button></div>}
        </div>
        <button className={`filter-button ${onlyMine ? 'filter-button-on' : ''}`} onClick={() => setOnlyMine(!onlyMine)}><UserRound size={15} />Только мои</button>
        {activeTrigger && <button className="active-filter-chip" onClick={() => onTag(activeTrigger)}>#{activeTrigger}<X size={13} /></button>}
        {activeClusterNames.map((name) => <button key={name} className="active-filter-chip" onClick={() => setClusterFilters((current) => current.filter((slug) => !clusters.find((cluster) => cluster.slug === slug && cluster.name === name)))}>{name}<X size={13} /></button>)}
      </div>
      <label className="sort-select"><span>Сортировка</span><select value={sortMode} onChange={(event) => setSortMode(event.target.value as SortMode)} aria-label="Сортировка шуток">{Object.entries(SORT_LABELS).map(([key, label]) => <option key={key} value={key}>{label}</option>)}</select><ChevronDown size={13} /></label>
    </div>

    <section className="explore-grid">
      <div className="panel galaxy-panel">
        <div className="panel-heading galaxy-heading"><div><div className="panel-title-line"><span className="orbit-mark"><span /></span><h2>Карта юмора</h2><span className="live-pill"><span />{allJokes.length ? 'В эфире' : 'Пока пусто'}</span></div><p>Каждая точка — шутка. Рядом те, что говорят на похожем языке.</p></div><button className="icon-button muted-icon-button" onClick={() => { if (allJokes.length) setSelectedJoke(allJokes[Math.floor(Math.random() * allJokes.length)]); }} disabled={!allJokes.length} aria-label="Случайная шутка"><Sparkles size={16} /></button></div>
        <div className="galaxy-stage"><Suspense fallback={<div className="galaxy-loading">Загружаем трёхмерную карту…</div>}><Galaxy jokes={galaxyJokes} selectedId={selectedJoke?.id} highlightedIds={allSearchResultIds} onSelect={setSelectedJoke} onHover={setHoveredJoke} /></Suspense>
          {hoveredJoke && <div className="galaxy-hover-card"><span className="cluster-dot" style={{ background: hoveredJoke.cluster.color }} /><span>{hoveredJoke.text.replace(/\n/g, ' ').slice(0, 88)}{hoveredJoke.text.length > 88 ? '…' : ''}</span><ArrowUpRight size={14} /></div>}
          <div className="galaxy-stat"><span className="galaxy-stat-number">{galaxyJokes.length.toLocaleString('ru-RU')}</span><span>точек в поле зрения</span></div>
        </div>
        <div className="galaxy-legend"><div className="legend-label">ТЕМЫ</div><div className="legend-items">{clusters.slice(0, 6).map((cluster) => <button key={cluster.slug} className={`legend-item ${clusterFilters.includes(cluster.slug) ? 'legend-item-muted' : ''}`} onClick={() => setClusterFilters((current) => current.includes(cluster.slug) ? current.filter((slug) => slug !== cluster.slug) : [...current, cluster.slug])}><span style={{ background: cluster.color }} />{cluster.name.split(/[, ]/)[0]}</button>)}</div><span className="legend-hint">нажмите, чтобы отфильтровать</span></div>
        {searchResponse && <div className="map-search-note"><span className="map-search-pulse" />{allSearchResultIds.size} шуток подсвечены по запросу «{submittedQuery}»</div>}
      </div>
      {selectedJoke ? <aside className="panel featured-panel">
        <div className="featured-header"><div><span className="section-kicker">НА ЭТОЙ ОРБИТЕ</span><span className="featured-counter">№ {String(Number(selectedJoke.id) || 1).padStart(2, '0')}</span></div><button onClick={() => onShare(selectedJoke.id)} aria-label="Поделиться"><Share2 size={15} /></button></div>
        <div className="featured-quote-mark">“</div>
        <button className="featured-text" onClick={() => onOpen(selectedJoke.id)}>{selectedJoke.text}</button>
        <div className="featured-cluster"><span style={{ background: selectedJoke.cluster.color }} />{selectedJoke.cluster.name}</div>
        <div className="featured-triggers">{selectedJoke.triggers.slice(0, 4).map((tag) => <button key={tag} onClick={() => onTag(tag)}>#{tag}</button>)}</div>
        <div className="featured-author"><button className="author-line" onClick={() => onProfile(selectedJoke.author.id)}><Avatar user={selectedJoke.author} size="normal" /><span><b>{selectedJoke.author.username}</b><small>Автор шутки</small></span></button><button className="text-link" onClick={() => onOpen(selectedJoke.id)}>Профиль шутки <ArrowUpRight size={13} /></button></div>
        <div className="featured-actions"><button className={`button button-secondary ${selectedJoke.liked ? 'button-liked' : ''}`} onClick={() => onLike(selectedJoke.id)}><Heart size={15} fill={selectedJoke.liked ? 'currentColor' : 'none'} />{selectedJoke.likesCount} нравится</button><button className={`icon-button ${selectedJoke.favorited ? 'favorite-on' : ''}`} onClick={() => onFavorite(selectedJoke.id)} aria-label="В избранное"><Bookmark size={16} fill={selectedJoke.favorited ? 'currentColor' : 'none'} /></button></div>
        <div className="featured-footnote"><span className="featured-footnote-dot" />Нажмите на точку в карте, чтобы открыть другую шутку</div>
      </aside> : <aside className="panel featured-panel featured-empty"><EmptyState title="Пока нет публикаций" text="Первая шутка появится здесь после публикации." button="Добавить шутку" onClick={onAdd} /></aside>}
    </section>

    <section className="feed-section">
      <div className="feed-heading"><div><span className="section-kicker">ИЗ ВСЕХ ОРБИТ</span><h2>{activeTrigger ? `Шутки по тегу «${activeTrigger}»` : clusterFilters.length ? 'Выбранные темы' : 'Свежие шутки'} <span className="feed-count">{filteredJokes.length}</span></h2></div><button className="button button-ghost" onClick={onAdd}><Plus size={15} />Поделиться своей</button></div>
      {!displayJokes.length ? allJokes.length === 0 ? <EmptyState title="Галактика ждёт первой шутки" text="Пока здесь нет пользовательских публикаций. Добавьте первую, чтобы начать собирать коллекцию." button="Добавить первую шутку" onClick={onAdd} /> : <EmptyState title="На этой орбите пока тихо" text="Снимите часть фильтров или добавьте первую шутку в тему." button="Сбросить фильтры" onClick={onClear} /> : <div className="feed-grid">{displayJokes.map((joke) => <JokeCard key={joke.id} joke={joke} onLike={onLike} onFavorite={onFavorite} onTag={onTag} onProfile={onProfile} onOpen={onOpen} onShare={onShare} />)}</div>}
      {visibleCount < filteredJokes.length && <div ref={sentinelRef} className="feed-sentinel"><span className="loading-orbit" /><span>Подтягиваем шутки из другой орбиты…</span><button onClick={() => setVisibleCount((count) => Math.min(count + 8, filteredJokes.length))}>Показать ещё</button></div>}
      {visibleCount >= filteredJokes.length && serverHasMore && <div ref={sentinelRef} className="feed-sentinel"><span className="loading-orbit" /><span>{loadingMore ? 'Загружаем следующую партию…' : 'Здесь есть ещё шутки, подгружаем.'}</span><button onClick={() => void onLoadMore()}>Загрузить ещё</button></div>}
      {visibleCount >= filteredJokes.length && !serverHasMore && filteredJokes.length > 8 && <div className="feed-end"><span>✦</span> Вы на краю этой орбиты. Пока что.</div>}
    </section>
  </>;
}

function SlidersHorizontalIcon() {
  return <span className="sliders-symbol"><i /><i /><i /></span>;
}

function StandupPage({ credits, setCredits, jokes, showToast, backendAvailable, authenticated, onTopUp, onVoice }: {
  credits: number; setCredits: React.Dispatch<React.SetStateAction<number>>; jokes: Joke[]; showToast: (message: string) => void;
  backendAvailable: boolean; authenticated: boolean; onTopUp: () => void; onVoice: (setter: (value: string) => void, current?: string) => void;
}) {
  const [story, setStory] = useState('');
  const [style, setStyle] = useState<string>('custom');
  const [themes, setThemes] = useState<string[]>([]);
  const [candidates, setCandidates] = useState<Joke[]>([]);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [generated, setGenerated] = useState('');
  const [isSuggesting, setIsSuggesting] = useState(false);
  const [isGenerating, setIsGenerating] = useState(false);
  const [hasGenerated, setHasGenerated] = useState(false);
  const storyRef = useRef<HTMLTextAreaElement>(null);

  const handleSuggest = async () => {
    if (!backendAvailable) { showToast('Подбор шуток доступен после подключения сервера.'); return; }
    if (!authenticated) { showToast('Войдите через Яндекс ID, чтобы подобрать шутки к своей истории.'); return; }
    if (story.trim().length < 12) { showToast('Добавьте чуть больше деталей — так подберём шутки точнее.'); storyRef.current?.focus(); return; }
    setIsSuggesting(true);
    try {
      const response = await suggestJokes(story);
      setThemes(response.themes ?? []);
      setCandidates(response.jokes ?? []);
      setSelectedIds((response.jokes ?? []).slice(0, 3).map((joke) => joke.id));
      if (!response.jokes?.length) showToast('В коллекции пока нет подходящих публикаций. Добавьте первую шутку.');
    } catch (error) {
      showToast(error instanceof Error ? error.message : 'Не удалось подобрать шутки.');
    } finally { setIsSuggesting(false); }
  };

  const syncCredits = async () => {
    if (!backendAvailable) return;
    try {
      const response = await fetch('/api/me');
      if (!response.ok) return;
      const payload = await response.json();
      if (payload?.authenticated) setCredits(Math.max(0, Number(payload.user?.standup_credits ?? 0)));
    } catch { /* Keep the optimistic balance until the API is reachable again. */ }
  };

  const handleGenerate = async () => {
    if (!backendAvailable) { showToast('Генерация доступна после подключения сервера.'); return; }
    if (!authenticated) { showToast('Войдите через Яндекс ID, чтобы сохранить монолог и списать генерацию.'); return; }
    if (credits < 1) { onTopUp(); return; }
    if (story.trim().length < 12) { showToast('Сначала расскажите свою историю.'); storyRef.current?.focus(); return; }
    setCredits((value) => value - 1);
    setGenerated(''); setHasGenerated(false); setIsGenerating(true);
    let received = '';
    try {
      const response = await fetch('/api/standup/generate', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ story, style, joke_ids: selectedIds }),
      });
      if (!response.ok) {
        const payload = await response.json().catch(() => null);
        setIsGenerating(false);
        if (response.status === 402) {
          setCredits(0);
          showToast(payload?.error ?? 'На балансе нет генераций.');
          onTopUp();
        } else {
          setCredits((value) => value + 1);
          showToast(payload?.error ?? 'Не удалось создать монолог. Генерация возвращена на баланс.');
        }
        return;
      }
      if (!response.body) throw new Error('Сервер не вернул поток генерации.');
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const events = buffer.split('\n\n');
        buffer = events.pop() ?? '';
        for (const event of events) {
          const data = event.split('\n').find((line) => line.startsWith('data:'))?.slice(5).trim();
          if (!data || data === '[DONE]') continue;
          try {
            const payload = JSON.parse(data);
            const chunk = payload.text ?? payload.delta ?? '';
            if (chunk) { received += chunk; setGenerated((text) => text + chunk); }
          } catch { received += data; setGenerated((text) => text + data); }
        }
      }
    } catch (error) {
      setIsGenerating(false);
      await syncCredits();
      if (received) {
        setHasGenerated(true);
        showToast('Поток прервался. Сохранённая часть монолога осталась на экране.');
      } else {
        showToast(error instanceof Error ? error.message : 'Не удалось связаться с сервером генерации.');
      }
      return;
    }
    if (!received) {
      setIsGenerating(false);
      await syncCredits();
      showToast('Сервер не вернул текст монолога. Баланс обновлён.');
      return;
    }
    await syncCredits();
    setIsGenerating(false); setHasGenerated(true);
    showToast('Монолог готов. Хорошего выступления!');
  };

  const toggleCandidate = (id: string) => setSelectedIds((current) => current.includes(id) ? current.filter((item) => item !== id) : [...current, id]);
  const copyMonologue = async () => {
    try { await navigator.clipboard.writeText(generated); showToast('Монолог скопирован в буфер обмена.'); }
    catch { showToast('Не получилось скопировать текст.'); }
  };

  return <section className="standup-page">
    <div className="studio-hero"><div className="studio-orbit-art"><span className="studio-orbit orbit-a" /><span className="studio-orbit orbit-b" /><span className="studio-orbit-core"><Sparkles size={21} /></span><i className="studio-star star-a" /><i className="studio-star star-b" /></div><div className="studio-hero-copy"><div className="eyebrow"><span className="eyebrow-line" /> ТВОРЧЕСКАЯ СТУДИЯ</div><h1>Ваша история.<br /><span>Ваш выход.</span></h1><p>Превратите неловкий случай, маленькую победу или поездку в лифте в монолог, который хочется рассказать.</p><div className="studio-hero-tags"><span><Sparkles size={13} />Подбор шуток по смыслу</span><span><Zap size={13} />Потоковая генерация</span></div></div><div className="studio-credit-card"><div className="credit-card-top"><span className="credit-card-icon"><Zap size={16} fill="currentColor" /></span><span>ВАШ БАЛАНС</span><button onClick={onTopUp} aria-label="Пополнить баланс"><Plus size={15} /></button></div><div className="credit-big-number">{credits}<small>генераций</small></div><div className="credit-progress"><span style={{ width: `${Math.min(100, credits * 2)}%` }} /></div><button className="credit-topup-link" onClick={onTopUp}>Пополнить баланс <ArrowRight size={14} /></button></div></div>

    <div className="studio-workspace">
      <div className="studio-main-column">
        <section className="panel story-panel">
          <div className="panel-heading"><div><div className="step-kicker"><span>01</span> ВАША ИСТОРИЯ</div><h2>С чего всё началось?</h2><p>Напишите как другу — редактор поможет найти смешное.</p></div><span className="optional-label">ЛИЧНОЕ ОСТАЁТСЯ ЛИЧНЫМ</span></div>
          <div className="story-input-wrap"><textarea ref={storyRef} value={story} onChange={(event) => setStory(event.target.value)} placeholder="Расскажите о случае, который вас удивил, разозлил или рассмешил… Например: «Заказал доставку на работу, но курьер позвонил, когда я был в лифте. В итоге мы оба приехали не туда…»" maxLength={3500} /><button className="story-voice" onClick={() => onVoice((value) => setStory((current) => current ? `${current} ${value}` : value))}><Mic size={15} />Диктовать</button><span className="character-count">{story.length} / 3500</span></div>
          <div className="story-actions"><span><LockKeyhole size={13} />История видна только вам</span><button className="button button-secondary" disabled={isSuggesting || story.trim().length < 12} onClick={() => void handleSuggest()}>{isSuggesting ? <LoaderCircle size={15} className="spin" /> : <Wand2 size={15} />}{isSuggesting ? 'Ищем темы…' : 'Подобрать шутки в тему'}<ArrowRight size={14} /></button></div>
        </section>

        <section className="panel matches-panel">
          <div className="panel-heading match-heading"><div><div className="step-kicker"><span>02</span> МАТЕРИАЛ</div><h2>Добавим точные попадания</h2><p>Выберите шутки, которые хотите вплести в выступление.</p></div><span className="selection-counter">{selectedIds.length} выбрано</span></div>
          {themes.length > 0 && <div className="theme-strip"><span>Ваша история про</span>{themes.map((theme) => <span className="theme-chip" key={theme}>{theme}</span>)}</div>}
          {candidates.length ? <div className="candidate-list">{candidates.map((joke) => <label key={joke.id} className={`candidate-row ${selectedIds.includes(joke.id) ? 'candidate-row-selected' : ''}`}><input type="checkbox" checked={selectedIds.includes(joke.id)} onChange={() => toggleCandidate(joke.id)} /><span className="custom-checkbox"><Check size={12} /></span><span className="candidate-copy"><span>{joke.text.replace(/\n/g, ' ')}</span><small><i style={{ background: joke.cluster.color }} />{joke.cluster.name}</small></span><span className="candidate-likes"><Heart size={12} />{joke.likesCount}</span></label>)}</div> : <div className="matches-empty"><span className="matches-empty-icon"><Compass size={19} /></span><div><b>Пока нет подобранных шуток</b><span>Расскажите историю выше — найдём общие темы.</span></div></div>}
          <button className="browse-more-jokes" onClick={() => { const local = rankLocally(story || 'повседневность', jokes, 'semantic').semantic.slice(0, 3); setCandidates((current) => [...current, ...local.filter((item) => !current.some((old) => old.id === item.id))]); }}>Посмотреть ещё шутки <ArrowRight size={14} /></button>
        </section>
      </div>

      <aside className="studio-side-column">
        <section className="panel style-panel"><div className="panel-heading"><div><div className="step-kicker"><span>03</span> ПОДАЧА</div><h2>Выберите настроение</h2><p>Не копируйте — найдите свою интонацию.</p></div></div><div className="style-list">{STYLE_PRESETS.map((preset) => <button key={preset.id} className={`style-option ${style === preset.id ? 'style-option-active' : ''}`} onClick={() => setStyle(preset.id)}><span className="style-avatar" style={{ '--style-color': preset.color } as React.CSSProperties}>{preset.initials}</span><span className="style-option-copy"><b>{preset.title}</b><small>{preset.subtitle}</small></span><span className="style-radio"><i /></span></button>)}</div><div className="style-description"><Info size={13} /><span>{STYLE_PRESETS.find((preset) => preset.id === style)?.description}</span></div><button className="button button-primary generate-button" onClick={() => void handleGenerate()} disabled={isGenerating || story.trim().length < 12}><Sparkles size={16} />{isGenerating ? 'Собираем ваш монолог…' : 'Создать монолог'}<span className="generate-cost"><Zap size={12} />1</span></button><div className="generation-footnote">Одна генерация спишется с баланса</div></section>
        <section className={`panel output-panel ${generated ? 'output-panel-active' : ''}`}><div className="output-heading"><div><span className="output-live"><i className={isGenerating ? 'output-dot-blink' : ''} />{isGenerating ? 'ПИШЕМ В РЕАЛЬНОМ ВРЕМЕНИ' : hasGenerated ? 'ВАШ МОНОЛОГ' : 'РЕЗУЛЬТАТ ПОЯВИТСЯ ЗДЕСЬ'}</span><h2>{hasGenerated ? 'Проверьте материал' : 'Сцена свободна'}</h2></div>{generated && <button className="icon-button" onClick={() => void copyMonologue()} aria-label="Скопировать монолог"><Copy size={15} /></button>}</div>{generated ? <div className="monologue-output">{generated}{isGenerating && <span className="typing-cursor">▍</span>}</div> : <div className="output-placeholder"><span className="output-placeholder-mark">“</span><p>Сначала история. Потом — хорошие шутки.<br />А пока здесь чистый лист.</p><div className="stage-lights"><i /><i /><i /></div></div>}{hasGenerated && <div className="output-actions"><button className="button button-secondary button-small" onClick={() => void handleGenerate()} disabled={isGenerating}><Sparkles size={14} />Ещё вариант · 1 генерация</button><span><Check size={13} />Можно доработать вручную</span></div>}</section>
      </aside>
    </div>
    <div className="studio-bottom-note"><Sparkles size={14} /><span>Хороший материал начинается с правды. Даже если она слегка преувеличена.</span><button onClick={() => onTopUp()}>Тарифы и генерации <ArrowRight size={13} /></button></div>
  </section>;
}

function FavoritesPage({ jokes, onLike, onFavorite, onTag, onProfile, onOpen, onShare, onExplore, authRequired, hasMore, loadingMore, onLoadMore }: {
  jokes: Joke[]; onLike: (id: string) => void; onFavorite: (id: string) => void; onTag: (tag: string) => void;
  onProfile: (id: string) => void; onOpen: (id: string) => void; onShare: (id: string) => void; onExplore: () => void;
  authRequired: boolean; hasMore: boolean; loadingMore: boolean; onLoadMore: () => Promise<void>;
}) {
  return <section className="collection-page"><div className="page-intro collection-intro"><div className="eyebrow"><span className="eyebrow-line" /> ВАША ЛИЧНАЯ ПОДБОРКА</div><div className="collection-title-row"><div><h1>Избранное<span className="title-star">✦</span></h1><p>Шутки, к которым хочется вернуться.</p></div><div className="collection-count"><Bookmark size={18} /><b>{jokes.length}</b><span>сохранено</span></div></div></div>{jokes.length ? <div className="feed-grid">{jokes.map((joke) => <JokeCard key={joke.id} joke={joke} onLike={onLike} onFavorite={onFavorite} onTag={onTag} onProfile={onProfile} onOpen={onOpen} onShare={onShare} />)}</div> : authRequired ? <div className="empty-state"><span className="empty-icon"><LockKeyhole size={20} /></span><h3>Войдите, чтобы увидеть избранное</h3><p>Сохранённые шутки привязаны к вашему аккаунту.</p><a className="button button-secondary button-small" href="/auth/yandex/login">Войти через Яндекс <ArrowRight size={14} /></a></div> : <EmptyState title="Здесь будет ваше смешное" text="Нажмите на закладку у понравившейся шутки — и она сохранится в этой подборке." button="Исследовать галактику" onClick={onExplore} />}{hasMore && <div className="feed-sentinel"><span>{loadingMore ? 'Загружаем сохранённые шутки…' : 'В коллекции есть ещё шутки.'}</span><button disabled={loadingMore} onClick={() => void onLoadMore()}>{loadingMore ? 'Загружаем…' : 'Показать ещё'}</button></div>}</section>;
}

function ProfileUnavailable({ profileId, backendAvailable }: { profileId: string; backendAvailable: boolean }) {
  const isOwnProfile = profileId === 'me';
  return <section className="profile-page"><div className="empty-state"><span className="empty-icon">{isOwnProfile ? <LockKeyhole size={20} /> : <UserRound size={20} />}</span><h3>{isOwnProfile ? backendAvailable ? 'Войдите, чтобы открыть профиль' : 'Профиль временно недоступен' : 'Профиль не найден'}</h3><p>{isOwnProfile ? backendAvailable ? 'После входа здесь появятся ваши публикации и сохранённые шутки.' : 'Подключите сервер, чтобы загрузить профиль.' : 'Публичный профиль появится здесь после первой публикации автора.'}</p>{isOwnProfile && backendAvailable && <a className="button button-secondary button-small" href="/auth/yandex/login">Войти через Яндекс <ArrowRight size={14} /></a>}</div></section>;
}

function ProfilePage({ user, isOwner, authenticated, onLogout, jokes, allJokes, onLike, onFavorite, onTag, onProfile, onOpen, onShare, onAdd, hasMore, loadingMore, onLoadMore }: {
  user: Author; isOwner: boolean; authenticated: boolean; onLogout: () => void; jokes: Joke[]; allJokes: Joke[]; onLike: (id: string) => void; onFavorite: (id: string) => void;
  onTag: (tag: string) => void; onProfile: (id: string) => void; onOpen: (id: string) => void; onShare: (id: string) => void; onAdd: () => void;
  hasMore: boolean; loadingMore: boolean; onLoadMore: () => Promise<void>;
}) {
  const [tab, setTab] = useState<'posts' | 'favorites'>('posts');
  const profileJokes = isOwner ? allJokes.filter((joke) => joke.author.id === user.id) : jokes;
  const visible = tab === 'favorites' && isOwner ? allJokes.filter((joke) => joke.favorited) : profileJokes;
  const likes = user.totalLikes ?? profileJokes.reduce((sum, joke) => sum + joke.likesCount, 0);
  const jokeCount = user.jokesCount ?? profileJokes.length;
  const joined = user.createdAt ? `В Галактике с ${['января', 'февраля', 'марта', 'апреля', 'мая', 'июня', 'июля', 'августа', 'сентября', 'октября', 'ноября', 'декабря'][new Date(user.createdAt).getMonth()]} ${new Date(user.createdAt).getFullYear()}` : 'Дата регистрации не указана';
  const shareProfile = async () => {
    try { await navigator.clipboard.writeText(`${window.location.origin}/user/${encodeURIComponent(user.id)}`); }
    catch { /* sharing from Web Share API below */ }
  };
  return <section className="profile-page"><div className="profile-cover"><span className="cover-orbit cover-orbit-a" /><span className="cover-orbit cover-orbit-b" /><span className="cover-glow" /><span className="cover-star cover-star-a">✦</span><span className="cover-star cover-star-b">✧</span><span className="cover-label">АВТОРСКАЯ ОРБИТА</span></div><div className="profile-identity"><div className="profile-avatar-wrap"><Avatar user={user} size="large" /></div><div className="profile-heading-copy"><div className="profile-name-row"><h1>{user.username}</h1>{isOwner && <span className="profile-owner-pill"><UserRound size={12} />Это вы</span>}</div><p>{user.bio || 'Описание не добавлено.'}</p><span className="profile-joined"><Clock3 size={13} />{joined}</span></div><div className="profile-actions"><button className="button button-secondary" onClick={() => { void shareProfile(); if (navigator.share) navigator.share({ title: `Профиль ${user.username}`, url: `${window.location.origin}/user/${user.id}` }).catch(() => undefined); }}><Share2 size={15} />Поделиться профилем</button>{isOwner && <>{authenticated ? <button type="button" className="profile-login-link" onClick={() => void onLogout()}>Выйти из аккаунта <ArrowUpRight size={12} /></button> : <a className="profile-login-link" href="/auth/yandex/login">Войти через Яндекс <ArrowUpRight size={12} /></a>}<button className="button button-primary" onClick={onAdd}><Plus size={15} />Новая шутка</button></>}</div></div><div className="profile-stats"><div><b>{jokeCount}</b><span>шуток</span></div><div><b>{likes.toLocaleString('ru-RU')}</b><span>лайков</span></div><div><b>{profileJokes.reduce((sum, joke) => sum + joke.favoritesCount, 0)}</b><span>сохранений</span></div></div><div className="profile-tabs"><button className={tab === 'posts' ? 'profile-tab-active' : ''} onClick={() => setTab('posts')}><FileText size={15} />Публикации <span>{jokeCount}</span></button>{isOwner && <button className={tab === 'favorites' ? 'profile-tab-active' : ''} onClick={() => setTab('favorites')}><Bookmark size={15} />Избранное <span>{allJokes.filter((joke) => joke.favorited).length}</span><LockKeyhole size={12} /></button>}</div>{visible.length ? <div className="feed-grid profile-feed">{visible.map((joke) => <JokeCard key={joke.id} joke={joke} onLike={onLike} onFavorite={onFavorite} onTag={onTag} onProfile={onProfile} onOpen={onOpen} onShare={onShare} />)}</div> : <EmptyState title={tab === 'favorites' ? 'В избранном пока пусто' : isOwner ? 'Ваша первая шутка ещё впереди' : 'Публикаций пока нет'} text={tab === 'favorites' ? 'Сохранённые шутки доступны только вам.' : 'Новая авторская шутка появится здесь.'} button={isOwner && tab === 'posts' ? 'Добавить шутку' : undefined} onClick={onAdd} />}{tab === 'posts' && hasMore && <div className="feed-sentinel"><span>{loadingMore ? 'Загружаем публикации…' : 'У автора есть ещё публикации.'}</span><button disabled={loadingMore} onClick={() => void onLoadMore()}>{loadingMore ? 'Загружаем…' : 'Показать ещё'}</button></div>}</section>;
}

function DetailPage({ joke, similar, onLike, onFavorite, onTag, onProfile, onOpen, onShare, onBack }: {
  joke: Joke; similar: Joke[]; onLike: (id: string) => void; onFavorite: (id: string) => void; onTag: (tag: string) => void;
  onProfile: (id: string) => void; onOpen: (id: string) => void; onShare: (id: string) => void; onBack: () => void;
}) {
  return <article className="detail-page"><button className="back-link" onClick={onBack}><ArrowRight size={14} className="back-arrow" />Назад к галактике</button><div className="detail-layout"><div className="detail-main"><div className="detail-kicker"><span className="cluster-dot" style={{ background: joke.cluster.color }} />{joke.cluster.name}<span>·</span><span>ШУТКА № {joke.id}</span></div><h1>{joke.text.replace(/\n/g, ' ')}</h1><div className="detail-byline"><button className="author-line" onClick={() => onProfile(joke.author.id)}><Avatar user={joke.author} size="normal" /><span><b>{joke.author.username}</b><small>Автор этой шутки</small></span></button><span className="detail-date"><Clock3 size={13} />{new Intl.DateTimeFormat('ru-RU', { day: 'numeric', month: 'long', year: 'numeric' }).format(new Date(joke.createdAt))}</span></div><div className="detail-actions"><button className={`button button-secondary ${joke.liked ? 'button-liked' : ''}`} onClick={() => onLike(joke.id)}><Heart size={16} fill={joke.liked ? 'currentColor' : 'none'} />Нравится · {joke.likesCount}</button><button className={`button button-secondary ${joke.favorited ? 'button-liked' : ''}`} onClick={() => onFavorite(joke.id)}><Bookmark size={15} fill={joke.favorited ? 'currentColor' : 'none'} />В избранное</button><button className="button button-ghost" onClick={() => onShare(joke.id)}><Share2 size={15} />Поделиться</button></div><div className="detail-tags-block"><span className="section-kicker">СМЫСЛОВЫЕ ТРИГГЕРЫ</span><div className="detail-tags">{joke.triggers.map((tag) => <button key={tag} onClick={() => onTag(tag)}>#{tag}<ArrowUpRight size={11} /></button>)}</div></div><div className="detail-cluster-card"><span className="cluster-dot" style={{ background: joke.cluster.color }} /><div><b>Кластер «{joke.cluster.name}»</b><p>{joke.cluster.description}</p></div><button onClick={() => onTag(`cluster:${joke.cluster.slug}`)} aria-label="Открыть кластер"><ArrowUpRight size={15} /></button></div></div><aside className="detail-side"><span className="detail-side-orbit"><span /></span><span className="detail-side-kicker">НЕБО БЕЗ КОНЦА</span><b>У каждой шутки есть своя орбита.</b><p>Переходите по смысловым тегам и открывайте соседние темы.</p><div className="detail-side-stats"><span><Heart size={14} />{joke.likesCount} оценок</span><span><Bookmark size={14} />{joke.favoritesCount} сохранений</span></div></aside></div><section className="similar-section"><div className="feed-heading"><div><span className="section-kicker">СХОЖАЯ ОРБИТА</span><h2>Похожие шутки <span className="feed-count">{similar.length}</span></h2></div><span className="similar-note"><Compass size={14} />по теме и настроению</span></div>{similar.length ? <div className="feed-grid">{similar.map((item) => <JokeCard key={item.id} joke={item} compact onLike={onLike} onFavorite={onFavorite} onTag={onTag} onProfile={onProfile} onOpen={onOpen} onShare={onShare} />)}</div> : <EmptyState title="Похожие шутки появятся скоро" text="Загляните в другие кластеры, пока эта орбита наполняется." />}</section></article>;
}

function AdminConsole({ jokes, setJokes, onlineMode, showToast }: { jokes: Joke[]; setJokes: React.Dispatch<React.SetStateAction<Joke[]>>; onlineMode: boolean; showToast: (message: string) => void }) {
  const [moderationJokes, setModerationJokes] = useState<Joke[]>(jokes);
  const [moderationMessage, setModerationMessage] = useState('');
  const [jokePage, setJokePage] = useState(1);
  const [jokeHasMore, setJokeHasMore] = useState(false);
  const [loadingJokes, setLoadingJokes] = useState(false);
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [payments, setPayments] = useState<any[]>([]);
  const [paymentsMessage, setPaymentsMessage] = useState('Проверяем права доступа…');
  const [job, setJob] = useState<{ id: string; name: string; progress: number; message: string } | null>(null);
  const [uploadBusy, setUploadBusy] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const loadModerationPage = useCallback(async (page: number, append: boolean) => {
    if (!onlineMode) {
      setModerationMessage('Каталог недоступен без сервера. Модерация откроется после запуска сервера и входа администратора.');
      return;
    }
    setLoadingJokes(true);
    try {
      const response = await fetch(`/api/admin/jokes?page=${page}&limit=30`);
      const payload = await response.json().catch(() => null);
      if (!response.ok) throw new Error(payload?.error ?? 'Не удалось загрузить список шуток.');
      const batch: Joke[] = Array.isArray(payload?.jokes) ? payload.jokes : [];
      setModerationJokes((current) => {
        if (!append) return batch;
        const merged = new Map(current.map((item) => [item.id, item]));
        batch.forEach((item) => merged.set(item.id, item));
        return [...merged.values()];
      });
      setJokes((current) => {
        const merged = new Map(current.map((item) => [item.id, item]));
        batch.forEach((item) => merged.set(item.id, item));
        return [...merged.values()];
      });
      setJokePage(payload.page ?? page);
      setJokeHasMore(Boolean(payload.has_more));
      setModerationMessage('');
    } catch (error) {
      setModerationMessage(error instanceof Error ? error.message : 'Не удалось загрузить список шуток.');
    } finally { setLoadingJokes(false); }
  }, [onlineMode, setJokes]);

  useEffect(() => { void loadModerationPage(1, false); }, [loadModerationPage]);

  useEffect(() => {
    if (!onlineMode) { setPaymentsMessage('История платежей появится после подключения сервера и входа администратора.'); return; }
    fetch('/api/admin/payments').then(async (response) => {
      const payload = await response.json();
      if (!response.ok) throw new Error(payload.error ?? 'Войдите с правами администратора.');
      setPayments(payload.payments ?? []);
      setPaymentsMessage('');
    }).catch((error) => setPaymentsMessage(error instanceof Error ? error.message : 'История платежей пока недоступна.'));
  }, [onlineMode]);

  useEffect(() => {
    if (!job?.id) return;
    const timer = window.setInterval(() => {
      fetch(`/api/admin/imports/${encodeURIComponent(job.id)}`).then((response) => response.ok ? response.json() : null).then((status) => {
        if (!status) return;
        const progress = status.total_rows ? Math.round(status.processed_rows * 100 / status.total_rows) : 0;
        const finished = status.status === 'completed' || status.status === 'failed';
        setJob((current) => current ? { ...current, progress: finished ? 100 : progress, message: status.status === 'completed' ? `Импорт завершён: обработано ${status.processed_rows} строк.${status.error ? ` Ошибки: ${status.error}` : ''}` : status.status === 'failed' ? `Импорт завершился с ошибкой: ${status.error || 'не удалось обработать файл.'}` : `Обработано ${status.processed_rows} из ${status.total_rows}${status.error ? ` · ${status.error}` : ''}` } : current);
        if (finished) window.clearInterval(timer);
      }).catch(() => undefined);
    }, 1300);
    return () => window.clearInterval(timer);
  }, [job?.id]);

  const upload = async () => {
    const file = selectedFile;
    if (!file) { showToast('Сначала выберите CSV-файл.'); return; }
    if (file.size > 10 * 1024 * 1024) { showToast('Размер CSV не должен превышать 10 МБ.'); return; }
    setUploadBusy(true);
    setJob({ id: '', name: file.name, progress: 3, message: 'Проверяем структуру файла…' });
    try {
      const form = new FormData(); form.append('file', file);
      const response = await fetch('/api/admin/import-csv', { method: 'POST', body: form });
      const payload = await response.json();
      if (!response.ok) throw new Error(payload.error ?? 'Нет доступа к импорту.');
      setJob({ id: String(payload.job_id), name: file.name, progress: 0, message: `В очереди ${payload.total_rows} строк.` });
      setSelectedFile(null);
      if (inputRef.current) inputRef.current.value = '';
      showToast('Импорт добавлен в фоновую очередь.');
    } catch (error) {
      setJob((current) => current ? { ...current, progress: 100, message: error instanceof Error ? error.message : 'Не удалось запустить импорт.' } : current);
    } finally { setUploadBusy(false); }
  };

  const adminRequest = async (joke: Joke, method: 'PATCH' | 'DELETE', body?: Record<string, unknown>) => {
    if (!onlineMode) throw new Error('Модерация доступна после запуска сервера.');
    const response = await fetch(`/api/admin/jokes/${encodeURIComponent(joke.id)}`, { method, headers: { 'Content-Type': 'application/json' }, body: body ? JSON.stringify(body) : undefined });
    const payload = await response.json().catch(() => null);
    if (!response.ok) throw new Error(payload?.error ?? 'Действие недоступно.');
    if (method === 'DELETE') {
      setModerationJokes((current) => current.filter((item) => item.id !== joke.id));
      setJokes((current) => current.filter((item) => item.id !== joke.id));
    } else {
      setModerationJokes((current) => current.map((item) => item.id === joke.id ? payload : item));
      setJokes((current) => current.map((item) => item.id === joke.id ? payload : item));
    }
    return payload;
  };
  const editJoke = async (joke: Joke) => {
    const text = window.prompt('Измените текст шутки:', joke.text);
    if (text === null || text.trim() === joke.text) return;
    try { await adminRequest(joke, 'PATCH', { text: text.trim(), triggers: joke.triggers, cluster: joke.cluster.slug }); showToast('Изменения сохранены.'); }
    catch (error) { showToast(error instanceof Error ? error.message : 'Не удалось изменить шутку.'); }
  };
  const toggleDuplicate = async (joke: Joke) => {
    try { await adminRequest(joke, 'PATCH', { is_duplicate: !joke.isDuplicate }); showToast(joke.isDuplicate ? 'Шутка снова видна в коллекции.' : 'Шутка помечена как возможный дубль.'); }
    catch (error) { showToast(error instanceof Error ? error.message : 'Не удалось обновить отметку.'); }
  };
  const deleteJoke = async (joke: Joke) => {
    if (!window.confirm('Удалить эту шутку без возможности восстановления?')) return;
    try { await adminRequest(joke, 'DELETE'); showToast('Шутка удалена.'); }
    catch (error) { showToast(error instanceof Error ? error.message : 'Не удалось удалить шутку.'); }
  };
  const paymentStatusLabel = (status: string) => ({ succeeded: 'Оплачен', pending: 'Ожидает оплаты', canceled: 'Отменён', failed: 'Ошибка оплаты' }[status] ?? 'Обрабатывается');
  const paymentPackageLabel = (code: string) => ({ pack_50: 'Пакет 50', pack_100: 'Пакет 100', pack_200: 'Пакет 200' }[code] ?? 'Пакет генераций');

  return <section className="admin-page"><div className="page-intro"><div className="eyebrow"><span className="eyebrow-line" /> УПРАВЛЕНИЕ ГАЛАКТИКОЙ</div><h1>Панель <span>редакции.</span></h1><p>Импортируйте коллекции, модерируйте дубликаты и следите за платежами.</p></div><div className="admin-notice"><Shield size={18} /><div><b>Доступ только для роли «администратор»</b><span>Действия защищены серверной проверкой. Назначьте роль в таблице users после входа через Яндекс ID.</span></div><a href="/auth/yandex/login" className="button button-secondary">Войти через Яндекс <ArrowUpRight size={14} /></a></div><div className="admin-grid"><section className="panel import-panel"><div className="step-kicker"><span>01</span> МАССОВЫЙ ИМПОРТ</div><h2>Загрузить CSV</h2><p>UTF-8, разделитель «;». Колонки: <code>id;text;triggers;cluster</code>.</p><input ref={inputRef} type="file" accept=".csv,text/csv" hidden onChange={(event) => setSelectedFile(event.target.files?.[0] ?? null)} /><button className="csv-dropzone" onClick={() => inputRef.current?.click()}><Upload size={23} /><b>{selectedFile?.name ?? 'Выберите CSV-файл'}</b><small>{selectedFile ? `${(selectedFile.size / 1024).toFixed(1)} КБ · готов к загрузке` : 'Максимальный размер — 10 МБ · до 50 000 строк'}</small></button><div className="import-meta"><span><Zap size={13} />До 15 шуток в секунду</span><span><Sparkles size={13} />Теги и эмбеддинги автоматически</span></div><button className="button button-primary import-button" disabled={uploadBusy || !selectedFile} onClick={() => void upload()}>{uploadBusy ? <LoaderCircle size={15} className="spin" /> : <Upload size={15} />}{uploadBusy ? 'Запускаем…' : 'Начать импорт'}<ArrowRight size={14} /></button>{job && <div className="import-result"><div className="progress-track"><span style={{ width: `${job.progress}%` }} /></div><b>{job.name}</b><br />{job.message}</div>}</section><section className="panel admin-queue"><div className="step-kicker"><span>02</span> СЛУЖЕБНАЯ ИНФОРМАЦИЯ</div><h2>Очередь и правила</h2><div className="admin-quick-stats"><div><b>{clusters.length}</b><span>кластеров по умолчанию</span></div><div><b>{moderationJokes.filter((item) => item.isDuplicate).length}</b><span>помечено как дубли</span></div></div><div className="queue-empty"><span><Clock3 size={17} /></span><b>Импорт работает в фоне</b><small>Статус задачи обновляется автоматически. Ошибки строк отображаются в карточке задания.</small></div></section></div>
    <section className="panel admin-moderation"><div className="admin-section-heading"><div><div className="step-kicker"><span>03</span> МОДЕРАЦИЯ</div><h2>Последние шутки</h2></div><span>{moderationJokes.length} шуток на странице · страница {jokePage}</span></div>{moderationMessage && <p className="admin-payment-message"><LockKeyhole size={14} />{moderationMessage}</p>}<div className="admin-joke-list">{moderationJokes.slice(0, 30).map((joke) => <div className={`admin-joke-row ${joke.isDuplicate ? 'admin-joke-duplicate' : ''}`} key={joke.id}><span className="cluster-dot" style={{ background: joke.cluster.color }} /><div className="admin-joke-copy"><b>{joke.text.replace(/\n/g, ' ')}</b><small>№ {joke.id} · {joke.cluster.name}{joke.isDuplicate ? ' · возможный дубль' : ''}</small></div><button className="admin-row-action" onClick={() => void editJoke(joke)}>Изменить</button><button className="admin-row-action" onClick={() => void toggleDuplicate(joke)}>{joke.isDuplicate ? 'Вернуть' : 'Дубль'}</button><button className="admin-row-delete" onClick={() => void deleteJoke(joke)} aria-label="Удалить шутку">×</button></div>)}</div>{jokeHasMore && <div className="admin-pagination"><button className="button button-secondary" disabled={loadingJokes} onClick={() => void loadModerationPage(jokePage + 1, true)}>{loadingJokes ? <LoaderCircle size={14} className="spin" /> : <ChevronDown size={14} />}Загрузить ещё шутки</button></div>}{loadingJokes && moderationJokes.length === 0 && <p className="admin-payment-message">Загружаем коллекцию…</p>}</section>
    <section className="panel admin-payments"><div className="admin-section-heading"><div><div className="step-kicker"><span>04</span> ОПЛАТЫ</div><h2>История платежей</h2></div><CreditCard size={17} /></div>{paymentsMessage ? <p className="admin-payment-message"><LockKeyhole size={14} />{paymentsMessage}</p> : payments.length ? <div className="payment-table">{payments.map((payment) => <div className="payment-row" key={payment.id}><span>{payment.username}</span><span>{paymentPackageLabel(payment.package_code)}</span><span>{payment.credits_amount} генераций</span><b>{payment.amount_rub} ₽</b><small>{paymentStatusLabel(payment.status)}</small></div>)}</div> : <p className="admin-payment-message">Платежей пока нет.</p>}</section>
  </section>;
}

function ModalFrame({ children, onClose, className = '' }: { children: React.ReactNode; onClose: () => void; className?: string }) {
  return <div className="modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}><div className={`modal-frame ${className}`} role="dialog" aria-modal="true">{children}</div></div>;
}

function OnboardingModal({ step, setStep, onClose }: { step: number; setStep: React.Dispatch<React.SetStateAction<number>>; onClose: () => void }) {
  const slide = ONBOARDING[step];
  const Icon = slide.icon;
  return <ModalFrame onClose={onClose} className="onboarding-modal"><button className="modal-close" onClick={onClose} aria-label="Закрыть"><X size={17} /></button><div className="onboarding-visual"><div className={`onboarding-orbit-art onboarding-art-${step}`}><span className="onboarding-ring ring-a" /><span className="onboarding-ring ring-b" /><span className="onboarding-core"><Icon size={23} /></span><i className="onboarding-spark spark-a">✦</i><i className="onboarding-spark spark-b">✧</i></div><div className="onboarding-micro-copy"><span>JOKEGALAXY</span><span>ВАША ОРБИТА ЮМОРА</span></div></div><View style={{ flex: 1 } as any}>
    <Text style={{ color: '#efc56e', fontSize: 10, fontWeight: '700', letterSpacing: 2.1, marginBottom: 13 }}>{slide.eyebrow}</Text>
    <Text style={{ color: '#f6f4fd', fontSize: 27, fontWeight: '700', lineHeight: 32, marginBottom: 14 }}>{slide.title}</Text>
    <Text style={{ color: '#a5a6bc', fontSize: 14, lineHeight: 23 }}>{slide.text}</Text>
    <div className="onboarding-progress">{ONBOARDING.map((item, index) => <button key={item.eyebrow} className={index === step ? 'progress-dot progress-dot-active' : 'progress-dot'} onClick={() => setStep(index)} aria-label={`Экран ${index + 1}`} />)}<span>{String(step + 1).padStart(2, '0')} <i>/ 04</i></span></div>
    <div className="onboarding-actions"><Pressable style={({ pressed }: any) => ({ opacity: pressed ? 0.82 : 1 })} onPress={step === ONBOARDING.length - 1 ? onClose : () => setStep((value) => value + 1)}><Text style={{ color: '#111320', fontSize: 13, fontWeight: '700' }}>{step === ONBOARDING.length - 1 ? 'Начать исследование' : 'Далее'} <span className="next-arrow">→</span></Text></Pressable><button onClick={onClose}>Пропустить</button></div>
  </View></ModalFrame>;
}

function AddJokeModal({ onClose, onSave, onToast, onlineMode, authRequired }: { onClose: () => void; onSave: (joke: Joke) => void; onToast: (message: string) => void; onlineMode: boolean; authRequired: boolean }) {
  const [text, setText] = useState('');
  const [clusterSlug, setClusterSlug] = useState(clusters[0].slug);
  const [triggers, setTriggers] = useState<string[]>([]);
  const [analyzing, setAnalyzing] = useState(false);
  const [saving, setSaving] = useState(false);
  const currentCluster = clusters.find((cluster) => cluster.slug === clusterSlug) ?? clusters[0];
  const analyze = async () => {
    if (text.trim().length < 8) { onToast('Добавьте немного контекста, чтобы ИИ понял шутку.'); return; }
    setAnalyzing(true);
    try {
      const response = await analyzeJoke(text);
      setTriggers(response.triggers ?? []);
      if (response.cluster) setClusterSlug(response.cluster);
    } catch {
      const words = text.toLocaleLowerCase('ru').match(/[а-яё]{4,}/gi) ?? [];
      setTriggers([...new Set(words)].slice(0, 7));
    } finally { setAnalyzing(false); }
  };
  const save = async () => {
    if (!onlineMode) { onToast('Публикация доступна после подключения сервера.'); return; }
    if (authRequired) { onToast('Войдите через Яндекс ID, чтобы опубликовать шутку.'); return; }
    if (text.trim().length < 8) { onToast('Сначала напишите шутку.'); return; }
    setSaving(true);
    try {
      const joke = await submitJoke(text.trim(), clusterSlug, triggers);
      if (!joke?.id || !joke.cluster) throw new Error('Сервер не подтвердил публикацию.');
      onSave(joke);
    } catch (error) {
      onToast(error instanceof Error ? error.message : 'Не удалось опубликовать шутку.');
    } finally { setSaving(false); }
  };
  return <ModalFrame onClose={onClose} className="add-modal"><div className="modal-header"><span className="modal-orbit-icon"><Plus size={17} /></span><div><span className="section-kicker">ПОПОЛНИТЬ ГАЛАКТИКУ</span><h2>Поделитесь смешным</h2></div><button className="modal-close-inline" onClick={onClose} aria-label="Закрыть"><X size={17} /></button></div><p className="modal-intro">Одна хорошая шутка — уже своя маленькая звезда.</p><label className="field-label" htmlFor="joke-text">Текст шутки</label><textarea id="joke-text" className="modal-textarea" value={text} onChange={(event) => setText(event.target.value)} placeholder="Запишите шутку целиком — с паузами, если они важны…" maxLength={1400} /><div className="textarea-foot"><button className="ai-analyze-link" onClick={() => void analyze()} disabled={analyzing}>{analyzing ? <LoaderCircle size={14} className="spin" /> : <Wand2 size={14} />}{analyzing ? 'Анализируем…' : 'Подобрать теги и кластер'}</button><span>{text.length} / 1400</span></div><label className="field-label" htmlFor="cluster-select">Кластер</label><div className="select-wrap"><span className="cluster-dot" style={{ background: currentCluster.color }} /><select id="cluster-select" value={clusterSlug} onChange={(event) => setClusterSlug(event.target.value)}>{clusters.map((cluster) => <option value={cluster.slug} key={cluster.slug}>{cluster.name}</option>)}</select><ChevronDown size={15} /></div><div className="cluster-description">{currentCluster.description}</div>{triggers.length > 0 && <div className="draft-trigger-wrap"><span className="field-label">ИИ предложил теги</span><div className="draft-triggers">{triggers.map((trigger) => <span key={trigger}>#{trigger}<button onClick={() => setTriggers((current) => current.filter((tag) => tag !== trigger))} aria-label={`Удалить тег ${trigger}`}><X size={11} /></button></span>)}</div></div>}{!onlineMode && <p className="auth-required-note"><LockKeyhole size={13} />Публикация появится после подключения сервера.</p>}{onlineMode && authRequired && <p className="auth-required-note"><LockKeyhole size={13} />Для публикации нужна авторизация. <a href="/auth/yandex/login">Войти через Яндекс ID</a></p>}<div className="modal-footer"><span><LockKeyhole size={13} />Публикуя, вы соглашаетесь с правилами сообщества</span><button className="button button-primary" onClick={() => void save()} disabled={saving || authRequired || text.trim().length < 8}>{saving ? <LoaderCircle size={15} className="spin" /> : <Sparkles size={15} />}{saving ? 'Публикуем…' : 'Опубликовать'}<ArrowRight size={14} /></button></div></ModalFrame>;
}

function CreditsModal({ onClose, credits, backendAvailable, authenticated, email, onToast }: { onClose: () => void; credits: number; backendAvailable: boolean; authenticated: boolean; email: string; onToast: (message: string) => void }) {
  const packages = [
    { code: 'pack_50', count: 50, price: 50, label: 'На пробу', featured: false },
    { code: 'pack_100', count: 100, price: 80, label: 'Больше генераций', featured: true },
    { code: 'pack_200', count: 200, price: 140, label: 'Для регулярных выходов', featured: false },
  ];
  const [pending, setPending] = useState('');
  const [receiptEmail, setReceiptEmail] = useState(email);
  const buy = async (item: typeof packages[number]) => {
    if (!backendAvailable) { onToast('Пополнение доступно после подключения сервера.'); return; }
    if (!authenticated) { onToast('Войдите через Яндекс ID, чтобы оплатить пакет генераций.'); return; }
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(receiptEmail.trim())) { onToast('Укажите email для электронного чека.'); return; }
    setPending(item.code);
    try {
      const response = await fetch('/api/payments/create', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ package_code: item.code, email: receiptEmail.trim() }) });
      const payload = await response.json().catch(() => null);
      if (response.ok && payload?.confirmation_url) { window.location.assign(payload.confirmation_url); return; }
      throw new Error(payload?.error ?? 'Не удалось создать платёж.');
    } catch (error) {
      onToast(error instanceof Error ? error.message : 'Не удалось связаться с ЮKassa.');
    } finally { setPending(''); }
  };
  return <ModalFrame onClose={onClose} className="credits-modal"><div className="modal-header"><span className="modal-orbit-icon credit-modal-icon"><Zap size={17} fill="currentColor" /></span><div><span className="section-kicker">ВАШ БАЛАНС · {credits} ГЕНЕРАЦИЙ</span><h2>Ещё немного сцены?</h2></div><button className="modal-close-inline" onClick={onClose} aria-label="Закрыть"><X size={17} /></button></div><p className="modal-intro">Одна генерация — один готовый вариант монолога. Выберите пакет, который подходит вашему темпу.</p>{!backendAvailable ? <p className="auth-required-note"><LockKeyhole size={13} />Оплата появится после подключения сервера.</p> : !authenticated ? <p className="auth-required-note"><LockKeyhole size={13} />Чтобы продолжить, <a href="/auth/yandex/login">войдите через Яндекс ID</a>.</p> : <><label className="field-label receipt-email-label" htmlFor="receipt-email">Email для электронного чека</label><input id="receipt-email" className="receipt-email-input" type="email" autoComplete="email" value={receiptEmail} onChange={(event) => setReceiptEmail(event.target.value)} placeholder="Введите email для чека" /><div className="package-list">{packages.map((item) => <button key={item.code} className={`package-option ${item.featured ? 'package-featured' : ''}`} onClick={() => void buy(item)} disabled={Boolean(pending)}><span className="package-radio"><i /></span><span className="package-copy"><b>{item.count} <small>генераций</small></b><span>{item.label}</span></span><span className="package-price">{pending === item.code ? <LoaderCircle size={17} className="spin" /> : <>{item.price}<small> ₽</small></>}</span>{item.featured && <span className="package-ribbon">ВЫГОДНЕЕ</span>}</button>)}</div><div className="payment-secure"><LockKeyhole size={13} /><span>Безопасная оплата через ЮKassa · электронный чек по 54-ФЗ</span><CreditCard size={16} /></div></>}{!authenticated && <div className="bonus-note"><Sparkles size={14} /><span>После входа новый аккаунт получит стартовые 50 генераций.</span></div>}</ModalFrame>;
}

export default App;
