import type { PersonSeed } from "@/components/avatar/traits";

export interface CastMember extends PersonSeed {
  name: string;
}

/** Made-up people. Sex is given for some and left out for others, as records vary. */
export const CAST: CastMember[] = [
  { key: "p-fatima-zaabi", name: "Fatima Al Zaabi", sex: "female" },
  { key: "p-khalid-hammadi", name: "Khalid Al Hammadi", sex: "male" },
  { key: "p-aisha-ketbi", name: "Aisha Al Ketbi", sex: "female" },
  { key: "p-maria-santos", name: "Maria Santos", sex: "female" },
  { key: "p-imran-qureshi", name: "Imran Qureshi", sex: "male" },
  { key: "p-noura-mansoori", name: "Noura Al Mansoori" },
  { key: "p-rajesh-nair", name: "Rajesh Nair", sex: "male" },
  { key: "p-mariam-shamsi", name: "Mariam Al Shamsi", sex: "female" },
  { key: "p-omar-suwaidi", name: "Omar Al Suwaidi", sex: "male" },
  { key: "p-hessa-dhaheri", name: "Hessa Al Dhaheri", sex: "female" },
  { key: "p-joseph-reyes", name: "Joseph Reyes" },
  { key: "p-saeed-nuaimi", name: "Saeed Al Nuaimi", sex: "male" },
  { key: "p-layla-haddad", name: "Layla Haddad", sex: "female" },
  { key: "p-ahmed-kaabi", name: "Ahmed Al Kaabi", sex: "male" },
  { key: "p-shaikha-mazrouei", name: "Shaikha Al Mazrouei", sex: "female" },
  { key: "p-ali-blooshi", name: "Ali Al Blooshi" },
  { key: "p-priya-menon", name: "Priya Menon", sex: "female" },
  { key: "p-hamad-falasi", name: "Hamad Al Falasi", sex: "male" },
  { key: "p-amna-qubaisi", name: "Amna Al Qubaisi", sex: "female" },
  { key: "p-yousef-darwish", name: "Yousef Darwish", sex: "male" },
  { key: "p-salma-rumaithi", name: "Salma Al Rumaithi" },
  { key: "p-tariq-hosani", name: "Tariq Al Hosani", sex: "male" },
  { key: "p-grace-okafor", name: "Grace Okafor", sex: "female" },
  { key: "p-sultan-mheiri", name: "Sultan Al Mheiri", sex: "male" },
];

export const FATIMA: PersonSeed = { key: "Fatima Al Zaabi", sex: "female" };
export const FATMA: PersonSeed = { key: "Fatma Al Zaabi", sex: "female" };
