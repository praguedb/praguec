using B2Tech.Feed.Cache.Core;
using Common.IntegratedEventApi.Models.Branded;
using Common.IntegratedEventApi.Models.Enums;
using Common.IntegratedEventApi.Models.VirtualGameEntity;
using CommonFeedModels.Enums;
using MessagePack;

namespace BO.Services.Prague.Models;

[MessagePackObject]
[DataCache(CacheClassName = nameof(BOSportGameCache))]
[DataCacheTopic("Cache.[v:brandId].SportGame")]
[DataCacheIndex(nameof(FixtureIdIndex), nameof(FixtureId), DataCacheIndexType.Many)]
public sealed partial class BOSportGameCacheItem
{
    [Key(0)]
    [DataCacheKey]
    public long EventId { get; set; }

    // Identifiers
    [Key(1)]
    public long SportTypeId { get; set; }

    [Key(2)]
    public long CountryId { get; set; }

    [Key(3)]
    public long LeagueId { get; set; }

    [Key(4)]
    public long? HomeTeamId { get; set; }

    [Key(5)]
    public long? AwayTeamId { get; set; }

    [Key(6)]
    public BOVenue? Venue { get; set; }

    [Key(7)]
    public long? StatisticsCenterId { get; set; }

    // Provider/meta
    [Key(8)]
    public string Name { get; set; } = string.Empty;

    [Key(9)]
    public string? ProviderSourceKey { get; set; }

    [Key(10)]
    public long ProviderKey { get; set; }

    [Key(11)]
    public byte ProviderId { get; set; }

    [Key(12)]
    public int EventTypeId { get; set; }

    [Key(14)]
    public int? FixtureId { get; set; }

    [DataCacheIndex]
    [IgnoreMember]
    public int FixtureIdIndex => FixtureId ?? 0;

    [Key(15)]
    public bool IsLive { get; set; }

    [Key(16)]
    public bool IsAllResulted { get; set; }

    [Key(17)]
    public bool IsAllSettled { get; set; }

    [Key(18)]
    public bool IsManualChanged { get; set; }

    [Key(19)]
    public bool? IsPinned { get; set; }

    [Key(20)]
    public bool IsContainsPendingOddsWithBets { get; set; }

    [Key(21)]
    [DataCacheIndex]
    public WatchDogAction WatchDogAction { get; set; }

    [Key(22)]
    public string? BetRadarEventStatus { get; set; }

    [Key(23)]
    [DataCacheIndex]
    public EventStatus EventStatus { get; set; } = EventStatus.Unknown;

    [Key(24)]
    public EventStatus? EventStatusManual { get; set; }

    [Key(25)]
    public EventStatus? EventStatusWatchDog { get; set; }

    [Key(26)]
    public BookingStatus BookingStatus { get; set; }

    [Key(27)]
    [DataCacheIndex(DataCacheIndexType.Range)]
    public DateTime EventDate { get; set; }

    [Key(28)]
    public DateTime? ClosedTime { get; set; }

    [Key(29)]
    public DateTime? EventDateManual { get; set; }

    [Key(30)]
    public DateTime? ResultedTime { get; set; }

    [Key(31)]
    public DateTime? SettledTime { get; set; }

    [Key(32)]
    public int Version { get; set; }

    [Key(33)]
    public long InternalTimestamp { get; set; }

    [Key(34)]
    [DataCacheIndex(DataCacheIndexType.Range)]
    public long ProviderTimestamp { get; set; }

    // Limits
    [Key(35)]
    public decimal? LiveLimitManualSingle { get; set; }

    [Key(36)]
    public decimal? PrematchLimitManualSingle { get; set; }

    [Key(37)]
    public decimal? LiveLimitManualCombo { get; set; }

    [Key(38)]
    public decimal? PrematchLimitManualCombo { get; set; }

    [Key(39)]
    public decimal? LiveBetBuilderManualLimit { get; set; }

    [Key(40)]
    public decimal? PrematchBetBuilderManualLimit { get; set; }

    // Counters and flags
    [Key(44)]
    public bool HasBetTypes { get; set; }

    [Key(45)]
    public bool IsSettlementBlocked { get; set; }

    [Key(46)]
    public List<Runner> Runners { get; set; } = [];

    // Coverage
    [Key(47)]
    public string? CoverageSource { get; set; }

    [Key(48)]
    public string? CoverageLevel { get; set; }

    [Key(49)]
    public int? PeriodLength { get; set; }

    [Key(50)]
    public DateTime? BetDataLastUpdateAt { get; set; }

    [Key(51)]
    public long EventsHubReceivedTimestampMs { get; set; }

    [Key(52)]
    public long EventsHubProduceTimestampMs { get; set; }
    
    [Key(53)]
    public int? EventSource { get; set; }
    
    [Key(54)]
    public StaticDataRef SportType { get; set; } = null!;
    
    [Key(55)]
    public StaticDataRef Country { get; set; } = null!;
    
    [Key(56)]
    public StaticDataRef League { get; set; } = null!;
    
    [Key(57)]
    public StaticDataRef? HomeTeam { get; set; } = null!;
    
    [Key(58)]
    public StaticDataRef? AwayTeam { get; set; } = null!;
}
