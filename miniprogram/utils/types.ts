// 与后端 JSON 字段对齐的 TS 类型定义

export interface Employee {
  staffId: number
  name: string
  title: string
  avatar: string
  phone: string
  /** 座机（本人办公电话）；为空时名片「座机」行回退公司总机 company.tel */
  landline: string
  email: string
  status: string
}

export interface CompanyInfo {
  fullName: string
  address: string
  tel: string
  logo: string
}

export interface CompanyProfile {
  introText: string
  videoChannelId: string
  backupMp4Url: string
  cover: string
}

export interface CardResp {
  employee?: Employee
  company?: CompanyInfo
  profile?: CompanyProfile
}

export interface Category {
  id: number
  name: string
  icon: string
  sort: number
}

export interface FeaturedProject {
  id: number
  name: string
  cover: string
  categoryId: number
}

export interface AchievementsResp {
  categories?: Category[]
  featured?: FeaturedProject[]
}

export interface ProjectListItem {
  id: number
  name: string
  cover: string
  summary: string
  categoryId: number
}

export interface ProjectDetail {
  id: number
  name: string
  summary: string
  detail: string // 富文本，含 <wx-video channel-id="..."> 块
  cover: string
  album: string[]
  categoryId: number
}

export interface ServiceListItem {
  id: number
  title: string
}

export interface ServiceDetail {
  id: number
  title: string
  content: string // 富文本，含视频号块
  updatedAt?: string
}

export interface HomeLayoutResp {
  order?: string[]
}
