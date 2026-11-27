import type { Author, Cluster, Joke, SearchMode, SortMode } from './types';

export const clusters: Cluster[] = [
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

const authors: Author[] = [
  { id: 'u-anna', username: 'Анна Лис', avatarUrl: 'https://api.dicebear.com/9.x/notionists/svg?seed=Anna&backgroundColor=ffd166', bio: 'Собираю смешное в обычных вещах.' },
  { id: 'u-maksim', username: 'Максим К.', avatarUrl: 'https://api.dicebear.com/9.x/notionists/svg?seed=Maksim&backgroundColor=45d9e8', bio: 'Разработчик днём, автор шуток после релиза.' },
  { id: 'u-vika', username: 'Вика Солнце', avatarUrl: 'https://api.dicebear.com/9.x/notionists/svg?seed=Vika&backgroundColor=ff6d9b', bio: 'Юмор как способ не открывать рабочую почту.' },
  { id: 'u-ilya', username: 'Илья Н.', avatarUrl: 'https://api.dicebear.com/9.x/notionists/svg?seed=Ilya&backgroundColor=6da9ff', bio: 'Люблю короткие истории с длинным послевкусием.' },
];

type Seed = { text: string; cluster: string; triggers: string[]; likes: number };
const seeds: Seed[] = [
  { text: '— Почему программист не любит природу?\n— Там слишком много багов, а исправлять их некому.', cluster: 'it', triggers: ['программист', 'баги', 'природа', 'работа'], likes: 148 },
  { text: 'На собеседовании спросили, умею ли я работать в команде.\nСказал: «Конечно. Я уже десять лет убеждаю себя, что мы справимся».', cluster: 'work-money', triggers: ['собеседование', 'команда', 'офис', 'самоирония'], likes: 97 },
  { text: '— Ты меня слушаешь?\n— Конечно. Просто у меня лицо загружается отдельно от мыслей.', cluster: 'relationships', triggers: ['отношения', 'разговор', 'внимание', 'семья'], likes: 84 },
  { text: 'Врач сказал: «Вам нужен покой».\nТеперь я не отвечаю даже на сообщения от банка. Лечусь комплексно.', cluster: 'medicine', triggers: ['врач', 'покой', 'здоровье', 'банк'], likes: 126 },
  { text: 'Купил ежедневник для планирования бюджета.\nТеперь я точно знаю, в какой день у меня закончатся деньги.', cluster: 'work-money', triggers: ['деньги', 'бюджет', 'планирование', 'зарплата'], likes: 113 },
  { text: '— Почему календарь такой спокойный?\n— У него все дни расписаны.', cluster: 'puns', triggers: ['календарь', 'расписание', 'двусмысленность', 'слова'], likes: 204 },
  { text: 'Учительница: «Кто назовёт два местоимения?»\nВовочка: «Кто, я?» — «Молодец, оба правильно!»', cluster: 'study-army', triggers: ['школа', 'местоимение', 'Вовочка', 'урок'], likes: 102 },
  { text: 'Штирлиц долго смотрел на карту. Карта смотрела в ответ.\nОба понимали: маршрут домой опять построен через ремонт.', cluster: 'heroes', triggers: ['Штирлиц', 'карта', 'ремонт', 'кино'], likes: 76 },
  { text: 'Оптимист видит свет в конце тоннеля.\nПессимист — поезд. Реалист — расписание задержек.', cluster: 'dark-philosophy', triggers: ['оптимист', 'тоннель', 'поезд', 'ирония'], likes: 165 },
  { text: '— Как понять, что сервер устал?\n— Он начинает отвечать: «Я подумаю над этим после перезагрузки».', cluster: 'it', triggers: ['сервер', 'усталость', 'перезагрузка', 'технологии'], likes: 92 },
  { text: 'В аптеке спросили: «Вам что-нибудь от нервов?»\n«Да. Скажите, что очередь движется быстрее, чем кажется».', cluster: 'medicine', triggers: ['аптека', 'очередь', 'нервы', 'ожидание'], likes: 68 },
  { text: 'В семье главное — уметь уступать.\nНапример, я уступил жене право выбирать фильм. Уже третий раз пересматриваем её выбор.', cluster: 'relationships', triggers: ['семья', 'фильм', 'компромисс', 'вечер'], likes: 88 },
  { text: 'Начальник сказал: «Нам нужна команда, которая горит работой».\nПосле этих слов офис проверили на пожарную безопасность.', cluster: 'work-money', triggers: ['начальник', 'команда', 'офис', 'выгорание'], likes: 132 },
  { text: 'Налоговая прислала письмо: «Мы вас нашли».\nЯ ответил: «А я вас и не терял».', cluster: 'politics', triggers: ['налоги', 'письмо', 'государство', 'переписка'], likes: 119 },
  { text: 'Студент так долго готовился к экзамену, что в итоге сдал не предмет, а надежду.', cluster: 'study-army', triggers: ['студент', 'экзамен', 'подготовка', 'учёба'], likes: 109 },
  { text: 'Дракон охранял сокровища так надёжно, что сам не мог вспомнить пароль от пещеры.', cluster: 'heroes', triggers: ['дракон', 'сказка', 'пароль', 'сокровища'], likes: 81 },
  { text: '— Ты уже взрослый?\n— По документам да. По ощущениям — всё ещё жду, когда придёт взрослый и всё объяснит.', cluster: 'dark-philosophy', triggers: ['взрослая жизнь', 'документы', 'ожидание', 'самоирония'], likes: 157 },
  { text: 'Программист поставил будильник на пять минут раньше.\nТеперь он опаздывает с запасом.', cluster: 'it', triggers: ['будильник', 'программист', 'время', 'опоздание'], likes: 73 },
  { text: 'Слова «всё под контролем» особенно убедительно звучат, когда их произносит человек с открытым зонтом в помещении.', cluster: 'puns', triggers: ['контроль', 'зонт', 'слова', 'абсурд'], likes: 61 },
  { text: 'Психолог спросил, умею ли я говорить «нет».\nЯ ответил: «Да, но только если вы не против».', cluster: 'medicine', triggers: ['психолог', 'границы', 'нет', 'вежливость'], likes: 95 },
  { text: 'Премия — это когда зарплата на секунду заходит в гости и оставляет чаевые.', cluster: 'work-money', triggers: ['премия', 'зарплата', 'деньги', 'офис'], likes: 141 },
  { text: 'В очереди за справкой я познакомился с человеком, который пришёл вчера.\nТеперь у меня есть друг и пример для подражания.', cluster: 'politics', triggers: ['очередь', 'справка', 'ожидание', 'бюрократия'], likes: 64 },
  { text: 'На контрольной учитель сказал: «Работаем самостоятельно».\nКласс впервые за год понял, что это не название предмета.', cluster: 'study-army', triggers: ['контрольная', 'учитель', 'самостоятельность', 'школа'], likes: 93 },
  { text: 'Смотрю фильм про путешествия во времени.\nОчень удобно: если сюжет не понравится, перемотаю до момента, когда ещё не включил.', cluster: 'heroes', triggers: ['кино', 'время', 'сюжет', 'путешествие'], likes: 117 },
  { text: 'Жизнь — это длинный список дел, который почему-то не заканчивается после того, как ты вычеркнул слово «жизнь».', cluster: 'dark-philosophy', triggers: ['жизнь', 'список дел', 'философия', 'ирония'], likes: 188 },
  { text: 'Умный дом сам заказал фильтр для воды.\nПохоже, теперь в семье только он умеет заботиться о будущем.', cluster: 'it', triggers: ['умный дом', 'покупки', 'технологии', 'семья'], likes: 86 },
  { text: '— Почему у слова «коротко» столько букв?\n— Чтобы оно успело высказаться.', cluster: 'puns', triggers: ['слова', 'буквы', 'коротко', 'каламбур'], likes: 224 },
  { text: 'На свидании я сказал: «Я люблю спонтанность».\nМы спонтанно обсудили, почему оба выбрали ресторан по отзывам.', cluster: 'relationships', triggers: ['свидание', 'спонтанность', 'ресторан', 'отношения'], likes: 91 },
];

export const demoUser: Author = {
  id: 'me', username: 'Космический гость', avatarUrl: 'https://api.dicebear.com/9.x/notionists/svg?seed=Cosmic&backgroundColor=ffd166', bio: 'Исследую орбиты хорошего настроения.',
};

export const jokes: Joke[] = seeds.map((seed, index) => {
  const cluster = clusters.find((item) => item.slug === seed.cluster) ?? clusters[0];
  const angle = index * 2.399963;
  const radius = 0.28 + (index % 7) * 0.105;
  return {
    id: String(index + 1),
    text: seed.text,
    triggers: seed.triggers,
    cluster,
    author: authors[index % authors.length],
    likesCount: seed.likes,
    favoritesCount: Math.round(seed.likes * (0.12 + (index % 4) * 0.035)),
    createdAt: new Date(Date.now() - index * 1000 * 60 * 60 * 9).toISOString(),
    posX: Math.cos(angle) * radius,
    posY: Math.sin(angle) * radius * 0.72,
    posZ: Math.sin(angle * 0.56) * radius,
  };
});

export const popularTriggers = ['программист', 'отношения', 'деньги', 'самоирония', 'работа', 'врач', 'школа', 'двусмысленность'];
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
