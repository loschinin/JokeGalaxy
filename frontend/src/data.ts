import type { Joke, SearchMode, SortMode } from './types';

// These are the application's taxonomy, not preloaded user content.
export const clusters = [
  { id: 1, name: 'Каламбуры и игра слов', slug: 'puns', color: '#FFD700', description: 'Омофоны, двусмысленности, искажённые слова и абсурдные факты.' },
  { id: 2, name: 'IT и программисты', slug: 'it', color: '#00E5FF', description: 'Разработка, баги, костыли, собеседования, админы и нейросети.' },
  { id: 3, name: 'Отношения, семья и секс', slug: 'relationships', color: '#FF4081', description: 'Свидания, семья, флирт и домашняя дипломатия.' },
  { id: 4, name: 'Врачи и здоровье', slug: 'medicine', color: '#00E676', description: 'Больницы, стоматологи, диеты, привычки и аптеки.' },
  { id: 5, name: 'Работа, деньги и офис', slug: 'work-money', color: '#FF9100', description: 'Начальники, зарплата, кредиты, коллеги и бизнес.' },
  { id: 6, name: 'Политика, общество и законы', slug: 'politics', color: '#FF5252', description: 'Общество, налоги, дороги, новости и повседневные законы.' },
  { id: 7, name: 'Школа, студенты и армия', slug: 'study-army', color: '#7C4DFF', description: 'Учителя, экзамены, студенты и армейские будни.' },
  { id: 8, name: 'Кино, герои и фольклор', slug: 'heroes', color: '#448AFF', description: 'Кино, сказки, животные, книги и поп-культура.' },
  { id: 9, name: 'Черный юмор и философия', slug: 'dark-philosophy', color: '#B0BEC5', description: 'Сарказм, цинизм, старость и экзистенциальная ирония.' },
];

export const defaultSearchMode: SearchMode = 'hybrid';
export const defaultSort: SortMode = 'newest';

export function rankLocally(query: string, list: Joke[], mode: SearchMode) {
  const tokens = query.toLocaleLowerCase('ru').split(/[^\p{L}\p{N}]+/u).filter((token) => token.length > 1);
  const lexical = list.map((joke) => {
    const haystack = `${joke.text} ${joke.triggers.join(' ')} ${joke.cluster.name}`.toLocaleLowerCase('ru');
    let score = 0;
    tokens.forEach((token) => {
      if (haystack.includes(token)) score += joke.triggers.some((tag) => tag.toLocaleLowerCase('ru').includes(token)) ? 0.35 : 0.23;
    });
    return { ...joke, relevance: Math.min(0.99, score + (score > 0 ? 0.32 : 0)) };
  }).filter((joke) => (joke.relevance ?? 0) > 0).sort((a, b) => (b.relevance ?? 0) - (a.relevance ?? 0));
  const semantic = list.map((joke) => {
    const haystack = `${joke.text} ${joke.triggers.join(' ')} ${joke.cluster.description}`.toLocaleLowerCase('ru');
    const overlap = tokens.filter((token) => haystack.includes(token)).length;
    const themeBonus = tokens.length > 0 && joke.triggers.some((tag) => tokens.some((token) => tag.includes(token) || token.includes(tag))) ? 0.3 : 0;
    const stable = ((Number(joke.id) * 37) % 17) / 100;
    const score = Math.min(0.94, 0.35 + overlap * 0.095 + themeBonus + stable);
    return { ...joke, relevance: score };
  }).sort((a, b) => (b.relevance ?? 0) - (a.relevance ?? 0));
  if (mode === 'text') return { keyword: lexical, semantic: [] as Joke[] };
  if (mode === 'semantic') return { keyword: [] as Joke[], semantic };
  const lexicalIds = new Set(lexical.map((joke) => joke.id));
  return { keyword: lexical, semantic: semantic.filter((joke) => !lexicalIds.has(joke.id)) };
}
