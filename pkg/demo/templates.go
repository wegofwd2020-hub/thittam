package demo

import "github.com/shopspring/decimal"

// verticalTemplate holds the demo-specific data for a vertical.
type verticalTemplate struct {
	CompanyName string
	Users       []DemoUser
	Projects    []DemoProject
}

// templates returns the demo data template for each vertical.
var templates = map[string]verticalTemplate{
	"movie-production": {
		CompanyName: "Sunrise Studios",
		Users: []DemoUser{
			{Name: "Rajesh Kumar", Email: "rajesh@{slug}.demo", Role: "super_admin", Department: "Management"},
			{Name: "Priya Sharma", Email: "priya@{slug}.demo", Role: "executive_producer", Department: "Production"},
			{Name: "Arun Nair", Email: "arun@{slug}.demo", Role: "line_producer", Department: "Production"},
			{Name: "Meena Iyer", Email: "meena@{slug}.demo", Role: "production_accountant", Department: "Finance"},
			{Name: "Vikram Reddy", Email: "vikram@{slug}.demo", Role: "production_manager", Department: "Production"},
			{Name: "Deepa Menon", Email: "deepa@{slug}.demo", Role: "crew_member", Department: "Camera"},
			{Name: "Karthik Rajan", Email: "karthik@{slug}.demo", Role: "crew_member", Department: "Art"},
			{Name: "Ananya Das", Email: "ananya@{slug}.demo", Role: "crew_member", Department: "Sound"},
		},
		Projects: []DemoProject{
			{Title: "The Last Horizon", Description: "Sci-fi thriller filming across Mumbai, Goa, and Ladakh", Phase: "production", BudgetTotal: decimal.NewFromInt(85000000), ExpenseCount: 20, Status: "active"},
			{Title: "Midnight Express Reboot", Description: "Modern reimagining of the classic action drama", Phase: "post_production", BudgetTotal: decimal.NewFromInt(120000000), ExpenseCount: 35, Status: "active"},
			{Title: "Project Starfall", Description: "Animated feature film targeting family audiences", Phase: "development", BudgetTotal: decimal.NewFromInt(40000000), ExpenseCount: 5, Status: "active"},
		},
	},
	"software-development": {
		CompanyName: "TechForge Solutions",
		Users: []DemoUser{
			{Name: "Arjun Mehta", Email: "arjun@{slug}.demo", Role: "super_admin", Department: "Management"},
			{Name: "Sneha Patel", Email: "sneha@{slug}.demo", Role: "executive_producer", Department: "Delivery"},
			{Name: "Rohan Gupta", Email: "rohan@{slug}.demo", Role: "line_producer", Department: "Engineering"},
			{Name: "Kavita Joshi", Email: "kavita@{slug}.demo", Role: "production_accountant", Department: "Finance"},
			{Name: "Amit Verma", Email: "amit@{slug}.demo", Role: "crew_member", Department: "Backend"},
			{Name: "Pooja Singh", Email: "pooja@{slug}.demo", Role: "crew_member", Department: "Frontend"},
			{Name: "Rahul Khanna", Email: "rahul@{slug}.demo", Role: "crew_member", Department: "QA"},
		},
		Projects: []DemoProject{
			{Title: "E-Commerce Platform v2", Description: "Fixed-price web application for retail client", Phase: "development", BudgetTotal: decimal.NewFromInt(5000000), ExpenseCount: 15, Status: "active"},
			{Title: "FinTech Mobile App", Description: "Cross-platform mobile app (React Native)", Phase: "qa", BudgetTotal: decimal.NewFromInt(8000000), ExpenseCount: 25, Status: "active"},
			{Title: "Cloud Migration — BFSI", Description: "T&M retainer for legacy migration", Phase: "go_live", BudgetTotal: decimal.NewFromInt(12000000), ExpenseCount: 40, Status: "active"},
			{Title: "HR SaaS Portal", Description: "Product development — monthly retainer", Phase: "discovery", BudgetTotal: decimal.NewFromInt(3000000), ExpenseCount: 3, Status: "active"},
		},
	},
	"construction": {
		CompanyName: "BuildRight Engineers",
		Users: []DemoUser{
			{Name: "Suresh Pillai", Email: "suresh@{slug}.demo", Role: "super_admin", Department: "Management"},
			{Name: "Lakshmi Rao", Email: "lakshmi@{slug}.demo", Role: "executive_producer", Department: "Projects"},
			{Name: "Mohan Das", Email: "mohan@{slug}.demo", Role: "line_producer", Department: "Site Management"},
			{Name: "Geeta Nair", Email: "geeta@{slug}.demo", Role: "production_accountant", Department: "Finance"},
			{Name: "Ravi Shankar", Email: "ravi@{slug}.demo", Role: "crew_member", Department: "Civil"},
			{Name: "Nandini Hegde", Email: "nandini@{slug}.demo", Role: "crew_member", Department: "MEP"},
		},
		Projects: []DemoProject{
			{Title: "Prestige Towers — Phase 2", Description: "32-floor residential tower in Bangalore", Phase: "structure", BudgetTotal: decimal.NewFromInt(450000000), ExpenseCount: 50, Status: "active"},
			{Title: "NH-48 Flyover Extension", Description: "2.5 km flyover extension — NHAI contract", Phase: "foundation", BudgetTotal: decimal.NewFromInt(180000000), ExpenseCount: 20, Status: "active"},
			{Title: "Tech Park — Building C", Description: "Commercial office space — Whitefield", Phase: "tender", BudgetTotal: decimal.NewFromInt(280000000), ExpenseCount: 0, Status: "active"},
		},
	},
	"events-management": {
		CompanyName: "Grandeur Events",
		Users: []DemoUser{
			{Name: "Nisha Kapoor", Email: "nisha@{slug}.demo", Role: "super_admin", Department: "Management"},
			{Name: "Sanjay Desai", Email: "sanjay@{slug}.demo", Role: "executive_producer", Department: "Accounts"},
			{Name: "Reema Choudhury", Email: "reema@{slug}.demo", Role: "line_producer", Department: "Production"},
			{Name: "Ajay Bhat", Email: "ajay@{slug}.demo", Role: "production_accountant", Department: "Finance"},
			{Name: "Divya Menon", Email: "divya@{slug}.demo", Role: "crew_member", Department: "AV & Technical"},
			{Name: "Kiran Shetty", Email: "kiran@{slug}.demo", Role: "crew_member", Department: "Logistics"},
		},
		Projects: []DemoProject{
			{Title: "TechSummit 2026 — Annual Conference", Description: "3-day corporate conference for 800 delegates", Phase: "production", BudgetTotal: decimal.NewFromInt(15000000), ExpenseCount: 30, Status: "active"},
			{Title: "Luxe Beauty — Product Launch", Description: "Press & influencer product launch event", Phase: "rehearsal", BudgetTotal: decimal.NewFromInt(5000000), ExpenseCount: 15, Status: "active"},
			{Title: "Sharma-Mehta Wedding", Description: "Full-service destination wedding — Udaipur", Phase: "planning", BudgetTotal: decimal.NewFromInt(25000000), ExpenseCount: 10, Status: "active"},
		},
	},
}
